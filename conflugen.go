package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	goconfluence "github.com/virtomize/confluence-go-api"
)

// Config — конфигурация запуска conflugen
type Config struct {
	ConfluenceURL string
	Token         string
	Files         []string
	DryRun        bool
	DebugMode     bool
}

// Run обрабатывает указанные файлы и синхронизирует их в Confluence
func Run(cfg Config) error {
	md := newMarkdownConverter()

	var client confluenceAPI
	var rawAPI rawRequester
	if !cfg.DryRun {
		c, err := goconfluence.NewAPI(cfg.ConfluenceURL, "", cfg.Token)
		if err != nil {
			return fmt.Errorf("create confluence client: %w", err)
		}
		goconfluence.SetDebug(cfg.DebugMode)
		client = c
		rawAPI = c
	}

	for _, filePath := range cfg.Files {
		content, err := os.ReadFile(filepath.Clean(filePath))
		if err != nil {
			return fmt.Errorf("read %s: %w", filePath, err)
		}

		directive, cleanedContent, err := ParseDirective(content)
		if err != nil {
			return fmt.Errorf("parse %s: %w", filePath, err)
		}

		if directive == nil {
			log.Printf("пропуск %s: нет директивы +conflugen", filePath)
			continue
		}

		pageTitle := directive.Title
		if pageTitle == "" {
			pageTitle = strings.TrimSuffix(filepath.Base(filePath), ".md")
		}

		htmlContent, contentHash, err := convertMarkdown(md, cleanedContent)
		if err != nil {
			return fmt.Errorf("convert %s: %w", filePath, err)
		}

		log.Printf("обработка: %s → %s (parent=%s, space=%s)",
			filePath, pageTitle, directive.ParentID, directive.SpaceKey)

		if cfg.DryRun {
			log.Printf("[DRY RUN] %s → страница \"%s\"", filePath, pageTitle)
			continue
		}

		if err := publishPage(
			client,
			rawAPI,
			cfg.ConfluenceURL,
			directive.ParentID,
			directive.SpaceKey,
			pageTitle,
			htmlContent,
			contentHash,
			cfg.DryRun,
		); err != nil {
			return fmt.Errorf("publish %s: %w", filePath, err)
		}
	}

	return nil
}
