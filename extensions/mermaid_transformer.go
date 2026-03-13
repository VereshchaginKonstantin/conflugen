package extensions

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

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
}

// NewMermaidHTMLRenderer создает новый рендерер
func NewMermaidHTMLRenderer(
	opts ...html.Option,
) renderer.NodeRenderer {
	r := &MermaidHTMLRenderer{
		Config: html.NewConfig(),
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

// renderMermaid рендерит Mermaid узел в макрос Confluence
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

	escapedContent := strings.ReplaceAll(
		mermaidNode.content, "]]>", "]]&gt;",
	)

	macro := fmt.Sprintf(
		`<ac:structured-macro ac:name="mermaid"`+
			` ac:schema-version="1">`+"\n"+
			`<ac:plain-text-body><![CDATA[%s]]>`+
			`</ac:plain-text-body>`+"\n"+
			`</ac:structured-macro>`,
		escapedContent,
	)

	_, _ = w.WriteString(macro)
	_, _ = w.WriteString("\n")
	return ast.WalkContinue, nil
}

// MermaidExtension добавляет расширение к Goldmark
type MermaidExtension struct{}

// Extend расширяет парсер и рендерер
func (e *MermaidExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(&MermaidTransformer{}, 100),
		),
	)

	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(NewMermaidHTMLRenderer(), 500),
		),
	)
}
