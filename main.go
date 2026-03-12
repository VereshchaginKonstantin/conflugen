package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
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
	var files arrayFlags

	flag.Var(&files, "f", "md файл для обработки (можно указать несколько раз)")
	token := flag.String("token", "", "Confluence API token (или env CONFLUENCE_TOKEN)")
	dryRun := flag.Bool("dry-run", false, "режим без изменений — только вывод плана")
	debug := flag.Bool("debug", false, "выводить отладочную информацию Confluence API")
	confluenceURL := flag.String("url", "", "URL Confluence REST API (или env CONFLUENCE_URL)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "conflugen — синхронизация Markdown → Confluence по директивам в файлах\n\n")
		fmt.Fprintf(os.Stderr, "Использование:\n")
		fmt.Fprintf(os.Stderr, "  conflugen -f file1.md -f file2.md [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Каждый md файл должен содержать директиву:\n")
		fmt.Fprintf(os.Stderr, "  <!-- +conflugen parent-id=123456 space-key=OB -->\n")
		fmt.Fprintf(os.Stderr, "  <!-- +conflugen title=\"Кастомный заголовок\" -->\n\n")
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

	apiToken := *token
	if apiToken == "" {
		apiToken = os.Getenv("CONFLUENCE_TOKEN")
	}

	if apiToken == "" && !*dryRun {
		fmt.Fprintln(os.Stderr, "ошибка: токен не указан (--token или CONFLUENCE_TOKEN)")
		fmt.Fprintln(os.Stderr, "для тестового запуска без токена используйте --dry-run")
		os.Exit(1)
	}

	apiURL := *confluenceURL
	if apiURL == "" {
		apiURL = os.Getenv("CONFLUENCE_URL")
	}

	if apiURL == "" {
		apiURL = defaultConfluenceURL
	}

	cfg := Config{
		ConfluenceURL: apiURL,
		Token:         apiToken,
		Files:         files,
		DryRun:        *dryRun,
		DebugMode:     *debug,
	}

	if err := Run(cfg); err != nil {
		log.Fatalf("conflugen: %v", err)
	}
}
