package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// defaultDownloadInterval — минимальный зазор между запросами при выгрузке.
// Больше публикаторских 300ms, потому что у download запросов на порядок
// больше: страница + её метки + список вложений + по запросу на каждый файл, и
// так на сотнях страниц. Плотная серия упирается в 429.
const defaultDownloadInterval = 2 * time.Second

// runDownload — точка входа подкоманды `conflugen download`.
// args — аргументы ПОСЛЕ слова download.
func runDownload(args []string) error {
	fs := flag.NewFlagSet("download", flag.ExitOnError)

	var lists arrayFlags
	fs.Var(&lists, "list", "файл со списком pageId, по одному на строку (можно указать несколько раз)")
	out := fs.String("out", "", "папка для выгрузки (обязательно)")
	force := fs.Bool("force", false, "перекачивать уже выгруженные страницы")
	token := fs.String("token", "", "Confluence Personal Access Token (или env CONFLUENCE_TOKEN)")
	user := fs.String("user", "", "Confluence username для basic auth (или env CONFLUENCE_USER)")
	password := fs.String("password", "", "Confluence пароль для basic auth (или env CONFLUENCE_PASSWORD)")
	confluenceURL := fs.String("url", "", "URL Confluence REST API (или env CONFLUENCE_URL)")
	userAgent := fs.String("user-agent", "", "User-Agent для исходящих запросов (или env CONFLUENCE_USER_AGENT)")
	debug := fs.Bool("debug", false, "выводить отладочную информацию Confluence API")
	requestInterval := fs.String("request-interval", "", "минимальный интервал между запросами, напр. 2s; 0 — выключить")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "conflugen download — выгрузка страниц Confluence в локальную папку\n\n")
		fmt.Fprintf(os.Stderr, "Использование:\n")
		fmt.Fprintf(os.Stderr, "  conflugen download --list pages.txt --out ./dump\n\n")
		fmt.Fprintf(os.Stderr, "Файл списка: по одному числовому pageId на строку, # — комментарий.\n")
		fmt.Fprintf(os.Stderr, "Обход идёт по дочерним страницам и по ссылкам из текста, без ограничений.\n")
		fmt.Fprintf(os.Stderr, "Остановка — Ctrl+C; повторный запуск продолжит с того же места.\n\n")
		fmt.Fprintf(os.Stderr, "Флаги:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if len(lists) == 0 {
		return fmt.Errorf("не указан --list (файл со списком pageId)")
	}
	if *out == "" {
		return fmt.Errorf("не указан --out (папка для выгрузки)")
	}

	rootIDs, err := readPageIDList(lists)
	if err != nil {
		return err
	}
	if len(rootIDs) == 0 {
		return fmt.Errorf("список страниц пуст")
	}

	apiUser := firstNonEmpty(*user, os.Getenv("CONFLUENCE_USER"))
	apiSecret := firstNonEmpty(*password, os.Getenv("CONFLUENCE_PASSWORD"), *token, os.Getenv("CONFLUENCE_TOKEN"))
	if apiSecret == "" {
		if apiUser != "" {
			return fmt.Errorf("пароль не указан (--password или CONFLUENCE_PASSWORD)")
		}
		return fmt.Errorf("токен не указан (--token или CONFLUENCE_TOKEN)")
	}

	apiURL := firstNonEmpty(*confluenceURL, os.Getenv("CONFLUENCE_URL"), defaultConfluenceURL)

	interval := defaultDownloadInterval
	if s := firstNonEmpty(*requestInterval, os.Getenv("CONFLUENCE_REQUEST_INTERVAL")); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("неверный --request-interval %q: %w", s, err)
		}
		interval = d
	}

	cfg := Config{
		ConfluenceURL:   apiURL,
		Username:        apiUser,
		Token:           apiSecret,
		UserAgent:       firstNonEmpty(*userAgent, os.Getenv("CONFLUENCE_USER_AGENT")),
		RequestInterval: interval,
		DebugMode:       *debug,
	}

	host := hostFromAPIURL(apiURL)
	if host == "" {
		return fmt.Errorf("не удалось разобрать хост из --url %q", apiURL)
	}

	authMode := "Bearer (PAT)"
	if apiUser != "" {
		authMode = "Basic (user=" + apiUser + ")"
	}

	// Баннер печатается ДО первого сетевого вызова. Если дальше него ничего не
	// появилось — процесс висит на HTTP: прокси, DNS, редирект или antibot.
	// Без этой строки молчание неотличимо от «программа ничего не делает».
	log.Printf("conflugen download")
	log.Printf("  url:      %s", apiURL)
	log.Printf("  auth:     %s", authMode)
	log.Printf("  out:      %s", *out)
	log.Printf("  интервал: %s между запросами", interval)
	log.Printf("  стартуем с %d страниц: %s", len(rootIDs), strings.Join(rootIDs, ", "))
	if *force {
		log.Printf("  --force: уже выгруженные страницы будут перекачаны")
	}

	api, raw, err := newConfluenceClient(cfg)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		return fmt.Errorf("создать папку выгрузки %s: %w", *out, err)
	}

	src := newPageSource(api, raw, apiURL)
	store := newPageStore(*out)
	c := newCrawler(src, store, host, *force)

	// Первый Ctrl+C отменяет контекст: краулер дописывает index.json и выходит.
	// stop() сразу после — возвращает сигналам поведение по умолчанию, чтобы
	// второй Ctrl+C убил процесс немедленно. Без явного stop() обработчик
	// остался бы зарегистрирован и второй сигнал был бы так же поглощён.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	defer stop()

	if err := c.Crawl(ctx, rootIDs); err != nil {
		return err
	}

	if err := store.FlushIndex(); err != nil {
		return err
	}

	if ctx.Err() != nil {
		os.Exit(130)
	}
	return nil
}

// readPageIDList читает файлы со списками pageId. Пустые строки и строки,
// начинающиеся с #, игнорируются. Нечисловая строка — ошибка, а не молчаливый
// пропуск: пользователь почти наверняка вставил URL и должен об этом узнать.
func readPageIDList(paths []string) ([]string, error) {
	var out []string

	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("открыть список %s: %w", path, err)
		}

		scanner := bufio.NewScanner(f)
		line := 0
		for scanner.Scan() {
			line++
			s := strings.TrimSpace(scanner.Text())
			if s == "" || strings.HasPrefix(s, "#") {
				continue
			}
			if !isDigits(s) {
				_ = f.Close()
				return nil, fmt.Errorf("%s:%d: %q не похоже на pageId — ожидается число", path, line, s)
			}
			out = append(out, s)
		}

		closeErr := f.Close()
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("читать список %s: %w", path, err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("закрыть список %s: %w", path, closeErr)
		}
	}

	return out, nil
}

// hostFromAPIURL достаёт хост (с портом, если есть) из URL REST API.
// Пустая строка означает, что URL неразбираем.
func hostFromAPIURL(apiURL string) string {
	u, err := url.Parse(apiURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// firstNonEmpty возвращает первую непустую строку — компактная замена лестнице
// из `if x == "" { x = … }`.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
