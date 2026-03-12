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

// PlantUMLTransformer преобразует блоки PlantUML в макрос Confluence
type PlantUMLTransformer struct{}

// Transform находит блоки кода с языком "plantuml" и заменяет их
func (t *PlantUMLTransformer) Transform(
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
		if strings.TrimSpace(strings.ToLower(language)) == "plantuml" {
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

		plantUMLNode := NewPlantUMLNode(fenced, reader)
		item.parent.InsertBefore(
			item.parent, item.node, plantUMLNode,
		)
		item.parent.RemoveChild(item.parent, item.node)
	}
}

// PlantUMLNode кастомный узел для PlantUML
type PlantUMLNode struct {
	ast.BaseBlock
	content string
}

// NewPlantUMLNode создает новый узел PlantUML
func NewPlantUMLNode(
	fenced *ast.FencedCodeBlock, reader text.Reader,
) *PlantUMLNode {
	lines := fenced.Lines()
	var content strings.Builder

	for i := range lines.Len() {
		line := lines.At(i)
		content.Write(line.Value(reader.Source()))
	}

	return &PlantUMLNode{
		content: strings.TrimSpace(content.String()),
	}
}

// Dump для отладки
func (n *PlantUMLNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Content": n.content,
	}, nil)
}

// KindPlantUML тип узла PlantUML
var KindPlantUML = ast.NewNodeKind("PlantUML")

// Kind возвращает тип узла
func (n *PlantUMLNode) Kind() ast.NodeKind {
	return KindPlantUML
}

// PlantUMLHTMLRenderer рендерер для PlantUML узлов
type PlantUMLHTMLRenderer struct {
	html.Config
}

// NewPlantUMLHTMLRenderer создает новый рендерер
func NewPlantUMLHTMLRenderer(
	opts ...html.Option,
) renderer.NodeRenderer {
	r := &PlantUMLHTMLRenderer{
		Config: html.NewConfig(),
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

// RegisterFuncs регистрирует функцию рендеринга
func (r *PlantUMLHTMLRenderer) RegisterFuncs(
	reg renderer.NodeRendererFuncRegisterer,
) {
	reg.Register(KindPlantUML, r.renderPlantUML)
}

// renderPlantUML рендерит PlantUML узел в макрос Confluence
func (r *PlantUMLHTMLRenderer) renderPlantUML(
	w util.BufWriter,
	_ []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	plantUMLNode, ok := node.(*PlantUMLNode)
	if !ok {
		return ast.WalkContinue, nil
	}

	escapedContent := strings.ReplaceAll(
		plantUMLNode.content, "]]>", "]]&gt;",
	)

	macro := fmt.Sprintf(
		`<ac:structured-macro ac:name="plantuml"`+
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

// PlantUMLExtension добавляет расширение к Goldmark
type PlantUMLExtension struct{}

// Extend расширяет парсер и рендерер
func (e *PlantUMLExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(&PlantUMLTransformer{}, 100),
		),
	)

	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(NewPlantUMLHTMLRenderer(), 500),
		),
	)
}
