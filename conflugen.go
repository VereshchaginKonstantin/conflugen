package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	goconfluence "github.com/virtomize/confluence-go-api"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
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
	mermaidCollector := extensions.NewMermaidCollector()
	md := newMarkdownConverter(mermaidCollector)

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

		mermaidCollector.Reset()

		htmlContent, contentHash, err := convertMarkdown(md, cleanedContent)
		if err != nil {
			return fmt.Errorf("convert %s: %w", filePath, err)
		}

		diagrams := mermaidCollector.Diagrams()

		log.Printf("обработка: %s → %s (parent=%s, space=%s)",
			filePath, pageTitle, directive.ParentID, directive.SpaceKey)

		if cfg.DryRun {
			log.Printf("[DRY RUN] %s → страница \"%s\"", filePath, pageTitle)
			if len(diagrams) > 0 {
				log.Printf("[DRY RUN] %d mermaid диаграмм будет загружено", len(diagrams))
			}
			continue
		}

		pageID, err := publishPage(
			client,
			rawAPI,
			cfg.ConfluenceURL,
			directive.ParentID,
			directive.SpaceKey,
			pageTitle,
			htmlContent,
			contentHash,
			cfg.DryRun,
		)
		if err != nil {
			return fmt.Errorf("publish %s: %w", filePath, err)
		}

		if len(diagrams) > 0 && pageID != "" {
			if err := uploadMermaidDiagrams(rawAPI, cfg.ConfluenceURL, pageID, diagrams); err != nil {
				return fmt.Errorf("upload mermaid diagrams for %s: %w", filePath, err)
			}
		}
	}

	return nil
}
