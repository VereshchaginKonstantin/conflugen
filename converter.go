package main

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

// newMarkdownConverter создаёт конвертер Markdown → Confluence HTML
func newMarkdownConverter() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			&extensions.PlantUMLExtension{},
			&extensions.ConfluenceCodeBlock{},
			&extensions.SpoilerExtension{},
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)
}

// convertMarkdown конвертирует markdown контент (без директив) в Confluence HTML с хешем
func convertMarkdown(md goldmark.Markdown, content []byte) (string, string, error) {
	var buf strings.Builder
	if err := md.Convert(content, &buf); err != nil {
		return "", "", fmt.Errorf("convert markdown: %w", err)
	}

	htmlContent := buf.String()
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(htmlContent)))

	return htmlContent, hash, nil
}

// annotateHTML добавляет подпись и хеш-макрос к HTML контенту
func annotateHTML(htmlContent, hash string) string {
	return htmlContent + "\n\n" +
		"<p>\n  <br/>\n</p>\n" +
		"<p><sub>conflugen-auto-generated:" + hash + "</sub></p>"
}
