package extensions

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const defaultSpoilerSummary = "Spoiler"

// SpoilerExtension создает расширение для конвертации спойлеров
type SpoilerExtension struct{}

func (e *SpoilerExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(NewSpoilerASTTransformer(), 999),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(NewSpoilerRenderer(), 999),
		),
	)
}

// NewSpoilerExtension создает новый экземпляр расширения
func NewSpoilerExtension() *SpoilerExtension {
	return &SpoilerExtension{}
}

// SpoilerBlock - узел для спойлера
type SpoilerBlock struct {
	ast.BaseBlock
	Summary string
}

// KindSpoilerBlock - тип узла для блока спойлера
var KindSpoilerBlock = ast.NewNodeKind("SpoilerBlock")

func (n *SpoilerBlock) Kind() ast.NodeKind {
	return KindSpoilerBlock
}

func (n *SpoilerBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Summary": n.Summary,
	}, nil)
}

// SpoilerASTTransformer преобразует HTML блоки в спойлеры
type SpoilerASTTransformer struct{}

func NewSpoilerASTTransformer() *SpoilerASTTransformer {
	return &SpoilerASTTransformer{}
}

func (t *SpoilerASTTransformer) Transform(
	node *ast.Document, reader text.Reader, _ parser.Context,
) {
	detailsPairs := t.collectDetailsPairs(node, reader)

	for _, pair := range detailsPairs {
		if pair.closeNode != nil ||
			strings.Contains(pair.content, "</details>") {
			transformDetailsPair(pair, reader)
		}
	}
}

func (t *SpoilerASTTransformer) collectDetailsPairs(
	node *ast.Document, reader text.Reader,
) []*detailsPair {
	var pairs []*detailsPair

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if htmlBlock, ok := n.(*ast.HTMLBlock); ok {
			content := getNodeContent(htmlBlock, reader)
			if strings.Contains(content, "<details") {
				pair := &detailsPair{
					openNode: htmlBlock,
					content:  content,
					children: []ast.Node{},
				}
				pairs = append(pairs, pair)
				return ast.WalkSkipChildren, nil
			}
		}

		if len(pairs) > 0 {
			t.processNodeForLastPair(pairs, n, reader)
		}

		return ast.WalkContinue, nil
	})

	return pairs
}

func (t *SpoilerASTTransformer) processNodeForLastPair(
	pairs []*detailsPair, n ast.Node, reader text.Reader,
) {
	lastPair := pairs[len(pairs)-1]

	if lastPair.closeNode != nil {
		return
	}

	if htmlBlock, ok := n.(*ast.HTMLBlock); ok {
		content := getNodeContent(htmlBlock, reader)
		if strings.Contains(content, "</details>") {
			lastPair.closeNode = htmlBlock
			lastPair.closeContent = content
			return
		}
	}

	if !isSummaryNode(n, reader) {
		lastPair.children = append(lastPair.children, n)
	}
}

type detailsPair struct {
	openNode     *ast.HTMLBlock
	closeNode    *ast.HTMLBlock
	content      string
	closeContent string
	children     []ast.Node
}

func transformDetailsPair(pair *detailsPair, reader text.Reader) {
	summary := extractSummary(pair.content)
	if summary == defaultSpoilerSummary && pair.closeContent != "" {
		summary = extractSummary(pair.closeContent)
	}

	spoiler := &SpoilerBlock{
		Summary: summary,
	}

	for _, child := range pair.children {
		if isSummaryNode(child, reader) {
			continue
		}

		if parent := child.Parent(); parent != nil {
			parent.RemoveChild(parent, child)
		}
		spoiler.AppendChild(spoiler, child)
	}

	if pair.openNode != nil {
		if parent := pair.openNode.Parent(); parent != nil {
			parent.ReplaceChild(parent, pair.openNode, spoiler)
		}
	}

	if pair.closeNode != nil {
		if parent := pair.closeNode.Parent(); parent != nil {
			parent.RemoveChild(parent, pair.closeNode)
		}
	}
}

// Получаем содержимое узла
func getNodeContent(n ast.Node, reader text.Reader) string {
	var buf bytes.Buffer

	switch node := n.(type) {
	case *ast.HTMLBlock:
		lines := node.Lines()
		for i := range lines.Len() {
			segment := lines.At(i)
			buf.Write(segment.Value(reader.Source()))
		}
	case *ast.RawHTML:
		for i := range node.Segments.Len() {
			segment := node.Segments.At(i)
			buf.Write(segment.Value(reader.Source()))
		}
	case *ast.String:
		buf.Write(node.Value)
	}

	return buf.String()
}

// Проверяет, является ли узел summary
func isSummaryNode(node ast.Node, reader text.Reader) bool {
	if rawHTML, ok := node.(*ast.RawHTML); ok {
		content := getNodeContent(rawHTML, reader)
		if strings.Contains(content, "<summary") {
			return true
		}
	}

	if textNode, ok := node.(*ast.String); ok {
		content := string(textNode.Value)
		if strings.Contains(content, "<summary") {
			return true
		}
	}

	return false
}

// Извлекает заголовок из summary тега
func extractSummary(content string) string {
	re := regexp.MustCompile(`<summary>\s*(.*?)\s*</summary>`)
	if matches := re.FindStringSubmatch(content); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	re2 := regexp.MustCompile(`<summary>([^<]*)</summary>`)
	if matches := re2.FindStringSubmatch(content); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	return defaultSpoilerSummary
}

// spoilerRenderer рендерит спойлеры в Confluence Storage Format
type spoilerRenderer struct {
	html.Config
}

func NewSpoilerRenderer() renderer.NodeRenderer {
	return &spoilerRenderer{
		Config: html.NewConfig(),
	}
}

func (r *spoilerRenderer) RegisterFuncs(
	reg renderer.NodeRendererFuncRegisterer,
) {
	reg.Register(KindSpoilerBlock, r.renderSpoilerBlock)
}

func (r *spoilerRenderer) renderSpoilerBlock(
	w util.BufWriter,
	_ []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		_, _ = w.WriteString(`</ac:rich-text-body>`)
		_, _ = w.WriteString(`</ac:structured-macro>`)
		_, _ = w.WriteString("\n")
		return ast.WalkContinue, nil
	}

	spoilerNode, ok := node.(*SpoilerBlock)
	if !ok {
		return ast.WalkContinue, nil
	}

	title := escapeXML(spoilerNode.Summary)

	_, _ = w.WriteString(
		`<ac:structured-macro ac:name="ui-expand"` +
			` ac:schema-version="1">`,
	)
	_, _ = w.WriteString(`<ac:parameter ac:name="title">`)
	_, _ = w.WriteString(title)
	_, _ = w.WriteString(`</ac:parameter>`)
	_, _ = w.WriteString(`<ac:rich-text-body>`)

	return ast.WalkContinue, nil
}

// SpoilerConverter для удобного использования
type SpoilerConverter struct {
	markdown goldmark.Markdown
}

func NewSpoilerConverter() *SpoilerConverter {
	md := goldmark.New(
		goldmark.WithExtensions(
			&SpoilerExtension{},
		),
	)

	return &SpoilerConverter{
		markdown: md,
	}
}

func (c *SpoilerConverter) Convert(source []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := c.markdown.Convert(source, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
