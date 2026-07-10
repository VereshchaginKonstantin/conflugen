package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	goconfluence "github.com/virtomize/confluence-go-api"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

// defaultUserAgent — User-Agent, который conflugen ставит на каждый исходящий
// запрос. Идентифицирует утилиту с URL — это сразу убирает «бот-баллы» у
// корпоративных antibot/WAF за анонимный `Go-http-client/1.1` и даёт админам
// чёткий маркер для allowlist'инга по UA, а не по IP.
const defaultUserAgent = "conflugen/0.1 (+https://github.com/VereshchaginKonstantin/conflugen)"

// defaultRequestInterval — минимальный интервал между исходящими запросами к
// Confluence по умолчанию. Confluence троттлит плотные серии запросов ответом
// 429 (Rate limit exceeded): на публикации одна страница даёт серию вызовов
// (GET версии и комментариев, PUT/POST контента, свойства, метки), и плотный
// пакет упирается в лимит. Этот зазор разносит запросы во времени.
// Переопределяется через --request-interval / CONFLUENCE_REQUEST_INTERVAL;
// значение 0 отключает троттлинг.
const defaultRequestInterval = 300 * time.Millisecond

// xsrfBypassTransport навешивает на каждый исходящий запрос заголовок
// X-Atlassian-Token: no-check, который отключает XSRF-проверку Confluence для
// state-changing REST-вызовов (POST/PUT/DELETE). Библиотека virtomize/confluence-go-api
// ставит этот заголовок только для аплоада вложений, поэтому без байпаса
// POST /content/ возвращает 403.
type xsrfBypassTransport struct {
	base http.RoundTripper
}

func (t *xsrfBypassTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("X-Atlassian-Token", "no-check")
	return t.base.RoundTrip(clone)
}

// userAgentTransport заменяет дефолтный `Go-http-client/1.1` (который ставит
// net/http) на идентифицирующий User-Agent. Если в запросе уже есть свой
// User-Agent — не трогает: даём пользователям возможность задать кастомный.
type userAgentTransport struct {
	userAgent string
	base      http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if clone.Header.Get("User-Agent") == "" {
		clone.Header.Set("User-Agent", t.userAgent)
	}
	return t.base.RoundTrip(clone)
}

// errorBodyLoggingTransport при получении ответа со статусом >= 400 печатает в
// stderr полный URL, метод, статус и тело ответа от Confluence (где почти всегда
// лежит JSON с полем message — настоящая причина 4xx/5xx). Библиотека goconfluence
// сама тело не показывает и сводит всё к "unknown response status: ...".
type errorBodyLoggingTransport struct {
	base http.RoundTripper
}

func (t *errorBodyLoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil || resp.StatusCode < 400 {
		return resp, err
	}

	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		log.Printf("HTTP %s %s → %s (не удалось прочитать тело ответа: %v)",
			req.Method, req.URL.Redacted(), resp.Status, readErr)
		resp.Body = io.NopCloser(bytes.NewReader(nil))
		return resp, nil
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		log.Printf("HTTP %s %s → %s (пустое тело)",
			req.Method, req.URL.Redacted(), resp.Status)
	} else {
		log.Printf("HTTP %s %s → %s\n%s",
			req.Method, req.URL.Redacted(), resp.Status, trimmed)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

// throttleTransport выдерживает минимальный интервал между исходящими HTTP-запросами
// к Confluence, чтобы не упираться в 429 (Rate limit exceeded) на плотных сериях.
// Зазор применяется ко ВСЕМ физическим запросам (GET, PUT, POST и hop'ам редиректов),
// потому что RoundTrip вызывается на каждый из них. interval <= 0 здесь не доходит:
// при выключенном троттлинге транспорт в цепочку не вставляется.
type throttleTransport struct {
	base     http.RoundTripper
	interval time.Duration
	mu       sync.Mutex
	last     time.Time
}

func (t *throttleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	if !t.last.IsZero() {
		if wait := t.interval - time.Since(t.last); wait > 0 {
			time.Sleep(wait)
		}
	}
	t.last = time.Now()
	t.mu.Unlock()
	return t.base.RoundTrip(req)
}

// Config — конфигурация запуска conflugen
type Config struct {
	ConfluenceURL string
	// Username — если задан, используется basic auth (username + Token как пароль);
	// если пуст — используется Bearer token (Personal Access Token).
	Username string
	Token    string
	// UserAgent — значение заголовка User-Agent на исходящих запросах. Пусто —
	// используется defaultUserAgent. Полезно переопределить, если корпоративный
	// antibot/WAF allowlist-ит конкретный UA.
	UserAgent string
	// RequestInterval — минимальный зазор между исходящими запросами к Confluence
	// (защита от 429). 0 или меньше — троттлинг выключен. Пусто в Config означает
	// 0; дефолт (defaultRequestInterval) подставляет вызывающий (main).
	RequestInterval time.Duration
	Files           []string
	DryRun          bool
	DebugMode       bool
}

// newConfluenceClient собирает клиент Confluence со всей транспортной цепочкой.
// Вынесен из Run, потому что подкоманда download нуждается ровно в том же
// клиенте: те же auth, cookie jar, XSRF-байпас, User-Agent и троттлинг.
//
// Цепочка: запрос → userAgent (ставит UA) → xsrf (добавляет header) → errLog →
// throttle → base (TCP/TLS). На обратном пути errLog видит ответ первым и
// печатает тело при 4xx/5xx. Троттлинг сидит ближе всего к сети, чтобы
// разносить каждый физический запрос, включая hop'ы редиректов.
func newConfluenceClient(cfg Config) (confluenceAPI, rawRequester, error) {
	c, err := goconfluence.NewAPI(cfg.ConfluenceURL, cfg.Username, cfg.Token)
	if err != nil {
		return nil, nil, fmt.Errorf("create confluence client: %w", err)
	}

	// Некоторые корпоративные edge-gateway/WAF перед Confluence на первом запросе
	// отвечают 307 с Set-Cookie, ожидая, что клиент пойдёт по редиректу уже с этой
	// cookie. Без CookieJar Go-клиент cookie теряет, gateway снова шлёт 307 на тот
	// же URL — Go упирается в лимит 10 редиректов.
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create cookie jar: %w", err)
	}
	c.Client.Jar = jar

	base := c.Client.Transport
	if base == nil {
		base = http.DefaultTransport
	}

	ua := cfg.UserAgent
	if ua == "" {
		ua = defaultUserAgent
	}

	netBase := base
	if cfg.RequestInterval > 0 {
		netBase = &throttleTransport{base: base, interval: cfg.RequestInterval}
	}

	c.Client.Transport = &userAgentTransport{
		userAgent: ua,
		base: &xsrfBypassTransport{
			base: &errorBodyLoggingTransport{base: netBase},
		},
	}

	goconfluence.SetDebug(cfg.DebugMode)
	return c, c, nil
}

// Run обрабатывает указанные файлы и синхронизирует их в Confluence
func Run(cfg Config) error {
	mermaidCollector := extensions.NewMermaidCollector()
	layoutCollector := extensions.NewLayoutCollector()
	imageCollector := extensions.NewImageCollector()
	md := newMarkdownConverter(mermaidCollector, layoutCollector, imageCollector)

	var client confluenceAPI
	var rawAPI rawRequester
	if !cfg.DryRun {
		var err error
		client, rawAPI, err = newConfluenceClient(cfg)
		if err != nil {
			return err
		}
	}

	for _, filePath := range cfg.Files {
		content, err := os.ReadFile(filepath.Clean(filePath))
		if err != nil {
			return fmt.Errorf("read %s: %w", filePath, err)
		}

		// Раскрываем `<!-- +conflugen-include path -->` до парса директив и goldmark:
		// фрагменты включаются как текст, директивы из них агрегируются ParseDirective,
		// goldmark видит цельный документ — все фичи (parent-by-title, ::: columns,
		// mermaid, спойлеры) работают одинаково в верхнем файле и во включённых.
		content, err = ExpandIncludes(filepath.Dir(filePath), content, nil)
		if err != nil {
			return fmt.Errorf("expand includes %s: %w", filePath, err)
		}

		// Собираем макросы (user-defined + stdlib-паки), удаляя соответствующие
		// директивы. Применим их после ParseDirective к cleanedContent.
		macros, content, err := ExtractMacros(content)
		if err != nil {
			return fmt.Errorf("extract macros %s: %w", filePath, err)
		}
		macros, content, err = EnableStdlibPacks(content, macros)
		if err != nil {
			return fmt.Errorf("stdlib packs %s: %w", filePath, err)
		}

		directive, cleanedContent, err := ParseDirective(content)
		if err != nil {
			return fmt.Errorf("parse %s: %w", filePath, err)
		}

		cleanedContent = ApplyMacros(cleanedContent, macros)
		cleanedContent = stripHTMLComments(cleanedContent)

		if directive == nil {
			log.Printf("пропуск %s: нет директивы +conflugen", filePath)
			continue
		}

		pageTitle := directive.Title
		// Имя файла как заголовок — только когда страница адресуется по
		// родителю (создание/поиск по title). При page-id пустой title
		// означает «сохранить текущий заголовок страницы» (см. updatePage).
		if pageTitle == "" && directive.PageID == "" {
			pageTitle = strings.TrimSuffix(filepath.Base(filePath), ".md")
		}
		titleForLog := pageTitle
		if titleForLog == "" {
			titleForLog = "page-id:" + directive.PageID
		}

		mermaidCollector.Reset()
		layoutCollector.Reset()
		imageCollector.Reset(filepath.Dir(filePath))

		htmlContent, contentHash, err := convertMarkdown(md, cleanedContent)
		if err != nil {
			return fmt.Errorf("convert %s: %w", filePath, err)
		}
		if err := layoutCollector.Err(); err != nil {
			return fmt.Errorf("convert %s: %w", filePath, err)
		}

		diagrams := mermaidCollector.Diagrams()
		images := imageCollector.Images()

		parentDesc := directive.ParentID
		if len(directive.Parents) > 0 {
			parentDesc = "by-title:" + strings.Join(directive.Parents, "/")
		}
		log.Printf("обработка: %s → %s (parent=%s, space=%s)",
			filePath, titleForLog, parentDesc, directive.SpaceKey)

		if cfg.DryRun {
			log.Printf("[DRY RUN] %s → страница \"%s\"", filePath, titleForLog)
			if len(diagrams) > 0 {
				log.Printf("[DRY RUN] %d mermaid диаграмм будет загружено", len(diagrams))
			}
			if len(images) > 0 {
				log.Printf("[DRY RUN] %d изображений будет загружено", len(images))
			}
			continue
		}

		if len(directive.Parents) > 0 {
			resolvedParent, err := ResolveParentID(confluenceAncestry{api: client}, directive.SpaceKey, directive.Parents)
			if err != nil {
				return fmt.Errorf("resolve ancestry for %s: %w", filePath, err)
			}
			directive.ParentID = resolvedParent
		}

		pageID, err := publishPage(
			client,
			rawAPI,
			cfg.ConfluenceURL,
			directive.PageID,
			directive.ParentID,
			directive.SpaceKey,
			pageTitle,
			htmlContent,
			contentHash,
			directive.Type,
			cfg.DryRun,
		)
		if err != nil {
			return fmt.Errorf("publish %s: %w", filePath, err)
		}

		if pageID != "" && len(directive.Labels) > 0 {
			if err := syncLabels(client, pageID, directive.Labels); err != nil {
				return fmt.Errorf("sync labels for %s: %w", filePath, err)
			}
		}

		if pageID != "" && directive.ContentAppearance != "" && rawAPI != nil {
			if err := setContentAppearance(rawAPI, cfg.ConfluenceURL, pageID, directive.ContentAppearance); err != nil {
				return fmt.Errorf("set content-appearance for %s: %w", filePath, err)
			}
		}

		if len(diagrams) > 0 && pageID != "" {
			if err := uploadMermaidDiagrams(rawAPI, cfg.ConfluenceURL, pageID, diagrams); err != nil {
				return fmt.Errorf("upload mermaid diagrams for %s: %w", filePath, err)
			}
		}

		if len(images) > 0 && pageID != "" {
			if err := uploadImageAttachments(rawAPI, cfg.ConfluenceURL, pageID, images); err != nil {
				return fmt.Errorf("upload images for %s: %w", filePath, err)
			}
		}
	}

	return nil
}
