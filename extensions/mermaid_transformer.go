package extensions

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// MermaidDiagram содержит имя файла и текст диаграммы для загрузки как attachment
type MermaidDiagram struct {
	Filename string
	Content  string
}

// MermaidCollector собирает диаграммы во время рендеринга
type MermaidCollector struct {
	mu       sync.Mutex
	diagrams []MermaidDiagram
}

// NewMermaidCollector создаёт новый коллектор
func NewMermaidCollector() *MermaidCollector {
	return &MermaidCollector{}
}

// Add добавляет диаграмму и возвращает имя файла
func (c *MermaidCollector) Add(content string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	filename := "mermaid-" + hash[:8]

	c.diagrams = append(c.diagrams, MermaidDiagram{
		Filename: filename,
		Content:  content,
	})

	return filename
}

// Diagrams возвращает собранные диаграммы
func (c *MermaidCollector) Diagrams() []MermaidDiagram {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]MermaidDiagram, len(c.diagrams))
	copy(result, c.diagrams)
	return result
}

// Reset очищает коллектор для повторного использования
func (c *MermaidCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.diagrams = c.diagrams[:0]
}

// MermaidTransformer преобразует блоки Mermaid в макрос Confluence
type MermaidTransformer struct{}

// Transform находит блоки кода с языком "mermaid" и заменяет их
func (t *MermaidTransformer) Transform(
	node *ast.Document, reader text.Reader, _ parser.Context,
) {
	var nodesToReplace []struct {
		parent ast.Node
		node   ast.Node
	}

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		fenced, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}

		language := string(fenced.Language(reader.Source()))
		if strings.TrimSpace(strings.ToLower(language)) == "mermaid" {
			nodesToReplace = append(nodesToReplace, struct {
				parent ast.Node
				node   ast.Node
			}{
				parent: n.Parent(),
				node:   n,
			})
		}

		return ast.WalkContinue, nil
	})

	for _, item := range nodesToReplace {
		if item.parent == nil {
			continue
		}

		fenced, ok := item.node.(*ast.FencedCodeBlock)
		if !ok {
			continue
		}

		mermaidNode := NewMermaidNode(fenced, reader)
		item.parent.InsertBefore(
			item.parent, item.node, mermaidNode,
		)
		item.parent.RemoveChild(item.parent, item.node)
	}
}

// MermaidNode кастомный узел для Mermaid
type MermaidNode struct {
	ast.BaseBlock
	content string
}

// NewMermaidNode создает новый узел Mermaid
func NewMermaidNode(
	fenced *ast.FencedCodeBlock, reader text.Reader,
) *MermaidNode {
	lines := fenced.Lines()
	var content strings.Builder

	for i := range lines.Len() {
		line := lines.At(i)
		content.Write(line.Value(reader.Source()))
	}

	return &MermaidNode{
		content: strings.TrimSpace(content.String()),
	}
}

// Dump для отладки
func (n *MermaidNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Content": n.content,
	}, nil)
}

// KindMermaid тип узла Mermaid
var KindMermaid = ast.NewNodeKind("Mermaid")

// Kind возвращает тип узла
func (n *MermaidNode) Kind() ast.NodeKind {
	return KindMermaid
}

// MermaidHTMLRenderer рендерер для Mermaid узлов
type MermaidHTMLRenderer struct {
	html.Config
	collector *MermaidCollector
}

// NewMermaidHTMLRenderer создает новый рендерер
func NewMermaidHTMLRenderer(
	collector *MermaidCollector, opts ...html.Option,
) renderer.NodeRenderer {
	r := &MermaidHTMLRenderer{
		Config:    html.NewConfig(),
		collector: collector,
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

// RegisterFuncs регистрирует функцию рендеринга
func (r *MermaidHTMLRenderer) RegisterFuncs(
	reg renderer.NodeRendererFuncRegisterer,
) {
	reg.Register(KindMermaid, r.renderMermaid)
}

// renderMermaid рендерит Mermaid узел в макрос Confluence mermaid-cloud
func (r *MermaidHTMLRenderer) renderMermaid(
	w util.BufWriter,
	_ []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	mermaidNode, ok := node.(*MermaidNode)
	if !ok {
		return ast.WalkContinue, nil
	}

	filename := r.collector.Add(mermaidNode.content)

	macro := fmt.Sprintf(
		`<ac:structured-macro ac:name="mermaid-cloud"`+
			` ac:schema-version="1">`+
			`<ac:parameter ac:name="filename">%s</ac:parameter>`+
			`<ac:parameter ac:name="toolbar">bottom</ac:parameter>`+
			`<ac:parameter ac:name="format">svg</ac:parameter>`+
			`<ac:parameter ac:name="zoom">fit</ac:parameter>`+
			`<ac:parameter ac:name="revision">1</ac:parameter>`+
			`</ac:structured-macro>`,
		filename,
	)

	_, _ = w.WriteString(macro)
	_, _ = w.WriteString("\n")
	return ast.WalkContinue, nil
}

// MermaidExtension добавляет расширение к Goldmark
type MermaidExtension struct {
	Collector *MermaidCollector
}

// Extend расширяет парсер и рендерер
func (e *MermaidExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(&MermaidTransformer{}, 100),
		),
	)

	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(NewMermaidHTMLRenderer(e.Collector), 500),
		),
	)
}
