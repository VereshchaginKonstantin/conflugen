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

// GliffyTransformer преобразует блоки gliffy в макрос Confluence
type GliffyTransformer struct{}

// Transform находит блоки кода с языком "gliffy" и заменяет их
func (t *GliffyTransformer) Transform(
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
		if strings.TrimSpace(strings.ToLower(language)) == "gliffy" {
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

		gliffyNode := NewGliffyNode(fenced, reader)
		item.parent.InsertBefore(
			item.parent, item.node, gliffyNode,
		)
		item.parent.RemoveChild(item.parent, item.node)
	}
}

// GliffyParam параметр макроса Gliffy
type GliffyParam struct {
	Name  string
	Value string
}

// GliffyNode кастомный узел для Gliffy
type GliffyNode struct {
	ast.BaseBlock
	params []GliffyParam
}

// NewGliffyNode создает новый узел Gliffy, парсит параметры из содержимого блока
func NewGliffyNode(
	fenced *ast.FencedCodeBlock, reader text.Reader,
) *GliffyNode {
	lines := fenced.Lines()
	var params []GliffyParam

	for i := range lines.Len() {
		line := lines.At(i)
		text := strings.TrimSpace(string(line.Value(reader.Source())))
		if text == "" {
			continue
		}

		parts := strings.SplitN(text, ":", 2)
		if len(parts) != 2 {
			continue
		}

		params = append(params, GliffyParam{
			Name:  strings.TrimSpace(parts[0]),
			Value: strings.TrimSpace(parts[1]),
		})
	}

	return &GliffyNode{params: params}
}

// Dump для отладки
func (n *GliffyNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// KindGliffy тип узла Gliffy
var KindGliffy = ast.NewNodeKind("Gliffy")

// Kind возвращает тип узла
func (n *GliffyNode) Kind() ast.NodeKind {
	return KindGliffy
}

// GliffyHTMLRenderer рендерер для Gliffy узлов
type GliffyHTMLRenderer struct {
	html.Config
}

// NewGliffyHTMLRenderer создает новый рендерер
func NewGliffyHTMLRenderer(opts ...html.Option) renderer.NodeRenderer {
	r := &GliffyHTMLRenderer{Config: html.NewConfig()}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

// RegisterFuncs регистрирует функцию рендеринга
func (r *GliffyHTMLRenderer) RegisterFuncs(
	reg renderer.NodeRendererFuncRegisterer,
) {
	reg.Register(KindGliffy, r.renderGliffy)
}

// renderGliffy рендерит Gliffy узел в макрос Confluence
func (r *GliffyHTMLRenderer) renderGliffy(
	w util.BufWriter,
	_ []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	gliffyNode, ok := node.(*GliffyNode)
	if !ok {
		return ast.WalkContinue, nil
	}

	_, _ = w.WriteString(`<ac:structured-macro ac:name="gliffy" ac:schema-version="1">` + "\n")
	for _, p := range gliffyNode.params {
		_, _ = w.WriteString(fmt.Sprintf(
			`<ac:parameter ac:name="%s">%s</ac:parameter>`+"\n",
			p.Name, p.Value,
		))
	}
	_, _ = w.WriteString("</ac:structured-macro>\n")

	return ast.WalkContinue, nil
}

// GliffyExtension добавляет расширение к Goldmark
type GliffyExtension struct{}

// Extend расширяет парсер и рендерер
func (e *GliffyExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(&GliffyTransformer{}, 100),
		),
	)

	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(NewGliffyHTMLRenderer(), 500),
		),
	)
}
