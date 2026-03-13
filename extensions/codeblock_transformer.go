package extensions

import (
	"html"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// ConfluenceCodeBlock расширение для конвертации кода в Confluence XML формат
type ConfluenceCodeBlock struct{}

// New создаёт новое расширение
func New() goldmark.Extender {
	return &ConfluenceCodeBlock{}
}

// Extend регистрирует расширение
func (e *ConfluenceCodeBlock) Extend(m goldmark.Markdown) {
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(&confluenceCodeBlockRenderer{}, 500),
	))
}

// confluenceCodeBlockRenderer рендерер для Confluence code blocks
type confluenceCodeBlockRenderer struct{}

// RegisterFuncs регистрирует функции рендеринга
func (r *confluenceCodeBlockRenderer) RegisterFuncs(
	reg renderer.NodeRendererFuncRegisterer,
) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
}

// renderFencedCodeBlock рендерит fenced code block
func (r *confluenceCodeBlockRenderer) renderFencedCodeBlock(
	w util.BufWriter, source []byte, node ast.Node, entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	codeBlock, ok := node.(*ast.FencedCodeBlock)
	if !ok {
		return ast.WalkContinue, nil
	}

	lang := r.getLanguage(source, codeBlock)

	diagramLanguages := map[string]bool{
		"plantuml": true,
		"puml":     true,
		"uml":      true,
		"mermaid":  true,
		"gliffy":   true,
	}

	if diagramLanguages[strings.ToLower(lang)] {
		return ast.WalkContinue, nil
	}
	content := r.getCodeContent(source, codeBlock)

	return r.renderXMLCodeBlock(w, lang, content)
}

// renderCodeBlock рендерит обычный code block
func (r *confluenceCodeBlockRenderer) renderCodeBlock(
	w util.BufWriter, source []byte, node ast.Node, entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	codeBlock, ok := node.(*ast.CodeBlock)
	if !ok {
		return ast.WalkContinue, nil
	}

	content := r.getCodeContent(source, codeBlock)

	return r.renderXMLCodeBlock(w, "", content)
}

// getLanguage извлекает язык программирования из fenced code block
func (r *confluenceCodeBlockRenderer) getLanguage(
	source []byte, codeBlock *ast.FencedCodeBlock,
) string {
	lang := codeBlock.Language(source)
	if lang == nil {
		return ""
	}
	return string(lang)
}

// getCodeContent извлекает содержимое кода
func (r *confluenceCodeBlockRenderer) getCodeContent(
	source []byte, codeBlock ast.Node,
) string {
	var content strings.Builder

	lines := codeBlock.Lines()
	for i := range lines.Len() {
		line := lines.At(i)
		content.Write(line.Value(source))
	}

	return content.String()
}

// renderXMLCodeBlock рендерит code block в XML формате Confluence
func (r *confluenceCodeBlockRenderer) renderXMLCodeBlock(
	w util.BufWriter, lang, content string,
) (ast.WalkStatus, error) {
	_, _ = w.WriteString(
		"<ac:structured-macro ac:name=\"code\">",
	)

	if lang != "" {
		_, _ = w.WriteString(
			"<ac:parameter ac:name=\"language\">",
		)
		_, _ = w.WriteString(html.EscapeString(lang))
		_, _ = w.WriteString("</ac:parameter>")
	}

	_, _ = w.WriteString("<ac:plain-text-body><![CDATA[")
	_, _ = w.WriteString(content)
	_, _ = w.WriteString("]]></ac:plain-text-body>")
	_, _ = w.WriteString("</ac:structured-macro>")

	return ast.WalkContinue, nil
}
