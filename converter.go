package main

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

// newMarkdownConverter создаёт конвертер Markdown → Confluence HTML
func newMarkdownConverter(mermaidCollector *extensions.MermaidCollector) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			&extensions.PlantUMLExtension{},
			&extensions.MermaidExtension{Collector: mermaidCollector},
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

// confluenceMacroRe находит экранированные Confluence макросы вида
// &lt;ac:structured-macro ...&gt;...&lt;/ac:structured-macro&gt;
// и self-closing &lt;ac:structured-macro .../&gt;
var confluenceMacroRe = regexp.MustCompile(
	`(?s)&lt;(/?ac:[a-z-]+)((?:\s+ac:[a-z-]+=&quot;[^&]*&quot;)*)(\s*/?)&gt;`,
)

// unescapeConfluenceMacros восстанавливает Confluence XML-макросы, экранированные goldmark.
// Goldmark не распознаёт XML namespace-теги (ac:structured-macro) как HTML
// и экранирует их в &lt;/&gt;. Эта функция возвращает их в исходный вид.
func unescapeConfluenceMacros(html string) string {
	// Убираем <p>...</p> обёртку вокруг макросов
	result := confluenceMacroRe.ReplaceAllStringFunc(html, func(match string) string {
		match = strings.ReplaceAll(match, "&lt;", "<")
		match = strings.ReplaceAll(match, "&gt;", ">")
		match = strings.ReplaceAll(match, "&quot;", "\"")
		return match
	})
	// Убираем <p> обёртку: <p><ac:...>...</ac:...></p> → <ac:...>...</ac:...>
	pWrapRe := regexp.MustCompile(`<p>\s*(<ac:[^<]*(?:<[^<]*)*</ac:[a-z-]+>)\s*</p>`)
	result = pWrapRe.ReplaceAllString(result, "$1")
	// Self-closing: <p><ac:... /></p>
	pWrapSelfRe := regexp.MustCompile(`<p>\s*(<ac:[a-z-]+[^>]*/\s*>)\s*</p>`)
	result = pWrapSelfRe.ReplaceAllString(result, "$1")
	return result
}

// convertMarkdown конвертирует markdown контент (без директив) в Confluence HTML с хешем
func convertMarkdown(md goldmark.Markdown, content []byte) (string, string, error) {
	var buf strings.Builder
	if err := md.Convert(content, &buf); err != nil {
		return "", "", fmt.Errorf("convert markdown: %w", err)
	}

	htmlContent := unescapeConfluenceMacros(buf.String())
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(htmlContent)))

	return htmlContent, hash, nil
}

// annotateHTML добавляет подпись и хеш-макрос к HTML контенту
func annotateHTML(htmlContent, hash string) string {
	return htmlContent + "\n\n" +
		"<p>\n  <br/>\n</p>\n" +
		"<p><sub>conflugen-auto-generated:" + hash + "</sub></p>"
}
