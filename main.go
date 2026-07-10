package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

const defaultConfluenceURL = "https://confluence.example.com/rest/api"

type arrayFlags []string

func (a *arrayFlags) String() string {
	return strings.Join(*a, ", ")
}

func (a *arrayFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}

func main() {
	// Подкоманда download разбирает свои флаги сама. Спутать её с именем
	// md-файла нельзя: файлы обязаны иметь суффикс .md.
	if len(os.Args) > 1 && os.Args[1] == "download" {
		if err := runDownload(os.Args[2:]); err != nil {
			log.Fatalf("conflugen download: %v", err)
		}
		return
	}

	var files arrayFlags

	flag.Var(&files, "f", "md файл для обработки (можно указать несколько раз)")
	token := flag.String("token", "", "Confluence Personal Access Token (или env CONFLUENCE_TOKEN); для Bearer auth")
	user := flag.String("user", "", "Confluence username для basic auth (или env CONFLUENCE_USER); пусто — Bearer token")
	password := flag.String("password", "", "Confluence пароль для basic auth (или env CONFLUENCE_PASSWORD); используется с --user")
	dryRun := flag.Bool("dry-run", false, "режим без изменений — только вывод плана")
	debug := flag.Bool("debug", false, "выводить отладочную информацию Confluence API")
	userAgent := flag.String("user-agent", "", "User-Agent для исходящих запросов (или env CONFLUENCE_USER_AGENT); пусто — идентифицирующий conflugen/<ver>")
	confluenceURL := flag.String("url", "", "URL Confluence REST API (или env CONFLUENCE_URL)")
	requestInterval := flag.String("request-interval", "", "минимальный интервал между запросами к Confluence от 429, напр. 300ms или 1s (или env CONFLUENCE_REQUEST_INTERVAL); пусто — дефолт, 0 — выключить")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "conflugen — синхронизация Markdown → Confluence по директивам в файлах\n\n")
		fmt.Fprintf(os.Stderr, "Использование:\n")
		fmt.Fprintf(os.Stderr, "  conflugen -f file1.md -f file2.md [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Подкоманды:\n")
		fmt.Fprintf(os.Stderr, "  conflugen download --list pages.txt --out ./dump   выгрузка страниц из Confluence\n\n")
		fmt.Fprintf(os.Stderr, "Каждый md файл должен содержать директиву:\n")
		fmt.Fprintf(os.Stderr, "  <!-- +conflugen parent-id=123456 space-key=OB -->\n")
		fmt.Fprintf(os.Stderr, "  <!-- +conflugen title=\"Кастомный заголовок\" -->\n\n")
		fmt.Fprintf(os.Stderr, "Аутентификация (выбери одну):\n")
		fmt.Fprintf(os.Stderr, "  Bearer:  --token PAT                       (или env CONFLUENCE_TOKEN)\n")
		fmt.Fprintf(os.Stderr, "  Basic:   --user LOGIN --password PASSWORD  (или CONFLUENCE_USER + CONFLUENCE_PASSWORD)\n\n")
		fmt.Fprintf(os.Stderr, "Флаги:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// Если файлы не переданы через -f, берём позиционные аргументы
	if len(files) == 0 {
		files = flag.Args()
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "ошибка: не указаны файлы для обработки")
		fmt.Fprintln(os.Stderr, "использование: conflugen -f file1.md -f file2.md")
		os.Exit(1)
	}

	apiUser := *user
	if apiUser == "" {
		apiUser = os.Getenv("CONFLUENCE_USER")
	}

	// Секрет: для basic auth (с --user) — пароль, иначе — PAT.
	// Приоритет: --password → CONFLUENCE_PASSWORD → --token → CONFLUENCE_TOKEN.
	apiSecret := *password
	if apiSecret == "" {
		apiSecret = os.Getenv("CONFLUENCE_PASSWORD")
	}
	if apiSecret == "" {
		apiSecret = *token
	}
	if apiSecret == "" {
		apiSecret = os.Getenv("CONFLUENCE_TOKEN")
	}

	if apiSecret == "" && !*dryRun {
		if apiUser != "" {
			fmt.Fprintln(os.Stderr, "ошибка: пароль не указан (--password или CONFLUENCE_PASSWORD)")
		} else {
			fmt.Fprintln(os.Stderr, "ошибка: токен не указан (--token или CONFLUENCE_TOKEN)")
		}
		fmt.Fprintln(os.Stderr, "для тестового запуска без креденшлов используйте --dry-run")
		os.Exit(1)
	}

	apiURL := *confluenceURL
	if apiURL == "" {
		apiURL = os.Getenv("CONFLUENCE_URL")
	}

	if apiURL == "" {
		apiURL = defaultConfluenceURL
	}

	apiUserAgent := *userAgent
	if apiUserAgent == "" {
		apiUserAgent = os.Getenv("CONFLUENCE_USER_AGENT")
	}

	// Интервал между запросами: флаг → env → дефолт. Явный 0 (например "0" или
	// "0s") выключает троттлинг.
	apiRequestInterval := *requestInterval
	if apiRequestInterval == "" {
		apiRequestInterval = os.Getenv("CONFLUENCE_REQUEST_INTERVAL")
	}
	interval := defaultRequestInterval
	if apiRequestInterval != "" {
		d, err := time.ParseDuration(apiRequestInterval)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ошибка: неверный --request-interval %q: %v\n", apiRequestInterval, err)
			os.Exit(1)
		}
		interval = d
	}

	cfg := Config{
		ConfluenceURL:   apiURL,
		Username:        apiUser,
		Token:           apiSecret,
		UserAgent:       apiUserAgent,
		RequestInterval: interval,
		Files:           files,
		DryRun:          *dryRun,
		DebugMode:       *debug,
	}

	if err := Run(cfg); err != nil {
		log.Fatalf("conflugen: %v", err)
	}
}
