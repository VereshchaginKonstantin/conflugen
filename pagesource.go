package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	goconfluence "github.com/virtomize/confluence-go-api"
)

// errAuth — креды не приняты. Фатально: если Confluence не пускает, следующие
// пятьсот страниц упадут ровно так же, и краулер обязан остановиться сразу, а
// не заполнить index.json пятьюстами одинаковых ошибок.
//
// Библиотека goconfluence на HTTP 401 возвращает нетипизированную ошибку с
// текстом "authentication failed" (см. request.go). Ловим по тексту — другого
// способа она не даёт.
var errAuth = errors.New("аутентификация не прошла")

// wrapAuthErr распознаёт 401 и подменяет ошибку на errAuth, сохраняя исходный текст.
func wrapAuthErr(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "authentication failed") {
		return fmt.Errorf("%w: %v", errAuth, err)
	}
	return err
}

// attachmentInfo — вложение страницы: как назвать файл и откуда его качать.
// DownloadPath относителен корню сайта, а не /rest/api.
type attachmentInfo struct {
	Title        string
	DownloadPath string
}

// fetchedPage — страница, полученная из Confluence: метаданные плюс тело.
// Attachments в Meta на этом этапе пуст: имена файлов вычисляет pageStore,
// краулер проставит их перед SaveMeta.
type fetchedPage struct {
	Meta    pageMeta
	Storage string
}

// pageSource — единственное место, знающее про Confluence. Про диск не знает
// ничего. Работает через интерфейсы confluenceAPI и rawRequester, поэтому
// целиком покрывается стабами.
type pageSource struct {
	api      confluenceAPI
	raw      rawRequester
	apiURL   string // …/rest/api
	siteRoot string // … (корень сайта, для ссылок на вложения)

	// resolved кэширует title+space → pageID на весь запуск: страница, на
	// которую ссылаются двадцать раз, резолвится один раз.
	resolved map[string]string
}

func newPageSource(api confluenceAPI, raw rawRequester, apiURL string) *pageSource {
	return &pageSource{
		api:      api,
		raw:      raw,
		apiURL:   strings.TrimSuffix(apiURL, "/"),
		siteRoot: siteRootFromAPIURL(apiURL),
		resolved: make(map[string]string),
	}
}

// siteRootFromAPIURL получает корень сайта из URL REST API. Нужен потому, что
// `_links.download` у вложения относителен корню сайта, а CONFLUENCE_URL
// указывает на …/rest/api. Для Cloud (…/wiki/rest/api) отсекается только
// суффикс /rest/api — /wiki остаётся частью корня.
func siteRootFromAPIURL(apiURL string) string {
	u := strings.TrimSuffix(apiURL, "/")
	return strings.TrimSuffix(u, "/rest/api")
}

// GetPage забирает страницу с телом, предками, пространством и версией, затем
// отдельным вызовом — метки. Отдельным, потому что goconfluence.Metadata меток
// не содержит (только Properties), и expand=metadata.labels типизированно не
// читается. Это +1 запрос на страницу; при интервале 2s цена приемлема.
func (s *pageSource) GetPage(pageID string) (fetchedPage, error) {
	content, err := s.api.GetContentByID(pageID, goconfluence.ContentQuery{
		Expand: []string{"body.storage", "version", "space", "ancestors"},
	})
	if err != nil {
		if authErr := wrapAuthErr(err); errors.Is(authErr, errAuth) {
			return fetchedPage{}, authErr
		}
		return fetchedPage{}, fmt.Errorf("получить страницу %s: %w", pageID, err)
	}

	meta := pageMeta{
		ID:    content.ID,
		Title: content.Title,
	}
	if content.Version != nil {
		meta.Version = content.Version.Number
	}
	if content.Space != nil {
		meta.SpaceKey = content.Space.Key
	}
	for _, a := range content.Ancestors {
		meta.AncestorIDs = append(meta.AncestorIDs, a.ID)
	}
	if content.Links != nil {
		meta.WebURL = content.Links.Base + content.Links.WebUI
	}

	// Метки — не критичны для дампа: если не отдались, продолжаем без них.
	if labels, err := s.api.GetLabels(pageID); err == nil && labels != nil {
		for _, l := range labels.Labels {
			meta.Labels = append(meta.Labels, l.Name)
		}
	}

	return fetchedPage{Meta: meta, Storage: content.Body.Storage.Value}, nil
}

// ListChildren возвращает id дочерних страниц. Обёртка вокруг GetChildPages,
// которая сама пагинирует внутри себя, — чтобы краулер не лез в поле s.api и
// граница «сеть здесь, диск там» держалась на методах, а не на честном слове.
func (s *pageSource) ListChildren(pageID string) ([]string, error) {
	res, err := s.api.GetChildPages(pageID)
	if err != nil {
		return nil, fmt.Errorf("дочерние страницы %s: %w", pageID, err)
	}
	if res == nil {
		return nil, nil
	}

	out := make([]string, 0, len(res.Results))
	for _, r := range res.Results {
		out = append(out, r.ID)
	}
	return out, nil
}

// attachmentListResponse — ответ GET /content/{id}/child/attachment.
// Своя структура, а не goconfluence.Attachment: библиотека не умеет
// перечислять вложения страницы, только получать одно по id.
type attachmentListResponse struct {
	Results []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Links struct {
			Download string `json:"download"`
		} `json:"_links"`
	} `json:"results"`
	Size  int `json:"size"`
	Limit int `json:"limit"`
}

const attachmentPageSize = 50

// ListAttachments перечисляет вложения страницы, проходя пагинацию.
func (s *pageSource) ListAttachments(pageID string) ([]attachmentInfo, error) {
	var out []attachmentInfo

	for start := 0; ; start += attachmentPageSize {
		url := fmt.Sprintf("%s/content/%s/child/attachment?limit=%d&start=%d",
			s.apiURL, pageID, attachmentPageSize, start)

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("запрос списка вложений %s: %w", pageID, err)
		}

		body, err := s.raw.Request(req)
		if err != nil {
			return nil, fmt.Errorf("список вложений %s: %w", pageID, wrapAuthErr(err))
		}

		var resp attachmentListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("разбор списка вложений %s: %w", pageID, err)
		}

		for _, r := range resp.Results {
			out = append(out, attachmentInfo{Title: r.Title, DownloadPath: r.Links.Download})
		}

		// Последняя страница пагинации: результатов меньше запрошенного лимита.
		if len(resp.Results) < attachmentPageSize {
			return out, nil
		}
	}
}

// DownloadAttachment качает тело вложения. downloadPath относителен корню сайта.
//
// Ограничение: rawRequester.Request читает ответ в память целиком, поэтому
// вложение в сотни мегабайт даст соответствующий пик RSS. Для документации с
// картинками и диаграммами это приемлемо.
func (s *pageSource) DownloadAttachment(downloadPath string) ([]byte, error) {
	url := s.siteRoot + downloadPath

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("запрос вложения %s: %w", url, err)
	}

	data, err := s.raw.Request(req)
	if err != nil {
		return nil, fmt.Errorf("скачать вложение %s: %w", url, wrapAuthErr(err))
	}
	return data, nil
}

// ResolveRef приводит pageRef к pageID. Ref с id возвращается как есть, без
// запроса. Ref по title+space ищется в Confluence, результат кэшируется.
// Ненайденная страница — не ошибка вызова, а пустой id: вызывающий решает,
// предупредить и пропустить.
func (s *pageSource) ResolveRef(ref pageRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.IsZero() {
		return "", nil
	}

	key := ref.Key()
	if id, ok := s.resolved[key]; ok {
		return id, nil
	}

	res, err := s.api.GetContent(goconfluence.ContentQuery{
		SpaceKey: ref.Space,
		Title:    ref.Title,
		Type:     "page",
		Limit:    1,
	})
	if err != nil {
		return "", fmt.Errorf("поиск страницы %q в пространстве %s: %w", ref.Title, ref.Space, wrapAuthErr(err))
	}

	id := ""
	if res != nil && len(res.Results) > 0 {
		id = res.Results[0].ID
	}

	s.resolved[key] = id
	return id, nil
}

// versionOf — маленький помощник для логов: "v7" либо "v?" если версии нет.
func versionOf(v int) string {
	if v == 0 {
		return "v?"
	}
	return "v" + strconv.Itoa(v)
}
