package extensions

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// ──────────────────────────── Узлы ────────────────────────────

var KindLayout = ast.NewNodeKind("ConfluenceLayout")
var KindLayoutSection = ast.NewNodeKind("ConfluenceLayoutSection")
var KindLayoutCell = ast.NewNodeKind("ConfluenceLayoutCell")
var KindLayoutMarker = ast.NewNodeKind("ConfluenceLayoutMarker")

// LayoutNode → <ac:layout> — обёртка всего тела страницы.
type LayoutNode struct{ ast.BaseBlock }

func (n *LayoutNode) Kind() ast.NodeKind         { return KindLayout }
func (n *LayoutNode) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

// LayoutSection → <ac:layout-section ac:type="…">.
type LayoutSection struct {
	ast.BaseBlock
	SectionType string // ac:type; "" до финализации в трансформере
	Synthetic   bool   // true для single-секции, обернувшей свободный контент
}

func (n *LayoutSection) Kind() ast.NodeKind { return KindLayoutSection }
func (n *LayoutSection) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"type": n.SectionType}, nil)
}

// LayoutCell → <ac:layout-cell>; содержит произвольные дочерние блоки.
type LayoutCell struct{ ast.BaseBlock }

func (n *LayoutCell) Kind() ast.NodeKind         { return KindLayoutCell }
func (n *LayoutCell) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

// markerKind — вид fence-маркера, который порождает block-parser.
type markerKind int

const (
	markerOpenColumns markerKind = iota
	markerOpenCell
	markerClose
)

// layoutMarker — однострочный leaf-узел, маркирующий fence ":::".
// Сборку section/cell делает layoutTransformer; маркеры после сборки удаляются.
type layoutMarker struct {
	ast.BaseBlock
	kind     markerKind
	typ      string // type= для open-columns
	parseErr error  // ошибка разбора info-string
}

func (n *layoutMarker) Kind() ast.NodeKind         { return KindLayoutMarker }
func (n *layoutMarker) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

// ──────────────────────────── Типы раскладки ────────────────────────────

// validLayoutTypes — допустимые ac:type Confluence и ожидаемое число ячеек.
var validLayoutTypes = map[string]int{
	"single":              1,
	"two_equal":           2,
	"two_left_sidebar":    2,
	"two_right_sidebar":   2,
	"three_equal":         3,
	"three_with_sidebars": 3,
}

// inferLayoutType выводит тип секции из числа ячеек, когда type= не задан.
func inferLayoutType(cellCount int) (string, error) {
	switch cellCount {
	case 1:
		return "single", nil
	case 2:
		return "two_equal", nil
	case 3:
		return "three_equal", nil
	default:
		return "", fmt.Errorf("layout: %d ячеек в секции — допустимо 1–3", cellCount)
	}
}

// ──────────────────────────── Разбор fence-строки ────────────────────────────

type fenceInfo struct {
	isFence bool   // строка начинается ровно с ":::"
	isClose bool   // голый ":::" без info-string
	kind    string // "columns" | "cell" | прочее
	typ     string // значение type= (только для columns)
	err     error  // ошибка разбора info-string
}

// parseFenceLine разбирает строку как pandoc-фенс ":::".
func parseFenceLine(line []byte) fenceInfo {
	s := strings.TrimRight(string(line), "\r\n")
	trimmed := strings.TrimLeft(s, " ")
	if !strings.HasPrefix(trimmed, ":::") {
		return fenceInfo{}
	}
	rest := trimmed[3:]
	if strings.HasPrefix(rest, ":") {
		// четыре и более двоеточий — не наш фенс
		return fenceInfo{}
	}
	info := strings.TrimSpace(rest)
	if info == "" {
		return fenceInfo{isFence: true, isClose: true}
	}
	fields := strings.Fields(info)
	fi := fenceInfo{isFence: true, kind: fields[0]}
	for _, f := range fields[1:] {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			fi.err = fmt.Errorf("layout: непонятный токен %q в %q", f, info)
			return fi
		}
		switch k {
		case "type":
			fi.typ = v
		default:
			fi.err = fmt.Errorf("layout: неизвестный атрибут %q", k)
			return fi
		}
	}
	return fi
}

// ──────────────────────────── Коллектор ошибок ────────────────────────────

// LayoutCollector копит ошибки валидации за один Convert (паттерн как MermaidCollector).
type LayoutCollector struct{ errs []error }

func NewLayoutCollector() *LayoutCollector { return &LayoutCollector{} }

func (c *LayoutCollector) Reset() { c.errs = nil }

func (c *LayoutCollector) Add(err error) {
	if err != nil {
		c.errs = append(c.errs, err)
	}
}

func (c *LayoutCollector) Err() error {
	if len(c.errs) == 0 {
		return nil
	}
	msgs := make([]string, len(c.errs))
	for i, e := range c.errs {
		msgs[i] = e.Error()
	}
	return fmt.Errorf("layout: %d ошибк(а/и):\n%s", len(c.errs), strings.Join(msgs, "\n"))
}

// ──────────────────────────── Block-parser ────────────────────────────

// layoutBlockParser порождает по одному layoutMarker на каждую fence-строку.
// Возврат ненулевого узла прерывает открытый параграф — иначе закрывающий ":::"
// был бы проглочен ленивым продолжением параграфа.
type layoutBlockParser struct{}

func (p *layoutBlockParser) Trigger() []byte { return []byte{':'} }

func (p *layoutBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	fi := parseFenceLine(line)
	if !fi.isFence {
		return nil, parser.NoChildren
	}
	m := &layoutMarker{}
	switch {
	case fi.isClose:
		m.kind = markerClose
	case fi.err != nil:
		m.parseErr = fi.err
	case fi.kind == "columns":
		m.kind = markerOpenColumns
		m.typ = fi.typ
	case fi.kind == "cell":
		m.kind = markerOpenCell
	default:
		m.parseErr = fmt.Errorf("layout: неизвестный блок '::: %s'", fi.kind)
	}
	reader.AdvanceToEOL()
	return m, parser.NoChildren
}

func (p *layoutBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	return parser.Close
}

func (p *layoutBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

func (p *layoutBlockParser) CanInterruptParagraph() bool { return true }
func (p *layoutBlockParser) CanAcceptIndentedLine() bool { return false }

// ──────────────────────────── AST-трансформер ────────────────────────────

type layoutTransformer struct{ collector *LayoutCollector }

func (t *layoutTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	// Снимок верхнего уровня в порядке документа.
	var nodes []ast.Node
	hasMarker := false
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		nodes = append(nodes, c)
		if c.Kind() == KindLayoutMarker {
			hasMarker = true
		}
	}
	if !hasMarker {
		return
	}

	// Сборка: проходим узлы, по маркерам строим section/cell.
	// items — упорядоченный список верхнего уровня: *LayoutSection или свободный узел.
	var items []ast.Node
	var curSection *LayoutSection
	var curCell *LayoutCell

	for _, n := range nodes {
		m, isMarker := n.(*layoutMarker)
		if !isMarker {
			switch {
			case curCell != nil:
				doc.RemoveChild(doc, n)
				curCell.AppendChild(curCell, n)
			case curSection != nil:
				t.collector.Add(fmt.Errorf("layout: контент между ячейками вне '::: cell'"))
			default:
				items = append(items, n) // свободный контент верхнего уровня
			}
			continue
		}

		if m.parseErr != nil {
			t.collector.Add(m.parseErr)
		}
		switch m.kind {
		case markerOpenColumns:
			if curSection != nil {
				t.collector.Add(fmt.Errorf("layout: вложенный '::: columns' не поддерживается Confluence"))
			} else {
				curSection = &LayoutSection{SectionType: m.typ}
				items = append(items, curSection)
			}
		case markerOpenCell:
			switch {
			case curSection == nil:
				t.collector.Add(fmt.Errorf("layout: '::: cell' вне '::: columns'"))
			case curCell != nil:
				t.collector.Add(fmt.Errorf("layout: предыдущая ячейка не закрыта перед новой"))
			default:
				curCell = &LayoutCell{}
				curSection.AppendChild(curSection, curCell)
			}
		case markerClose:
			switch {
			case curCell != nil:
				curCell = nil
			case curSection != nil:
				curSection = nil
			default:
				t.collector.Add(fmt.Errorf("layout: закрывающий ':::' без открытого блока"))
			}
		}
		doc.RemoveChild(doc, m)
	}

	if curCell != nil {
		t.collector.Add(fmt.Errorf("layout: незакрытый '::: cell' (нет ':::')"))
	}
	if curSection != nil {
		t.collector.Add(fmt.Errorf("layout: незакрытый '::: columns' (нет ':::')"))
	}

	// Финализация типов секций.
	for _, it := range items {
		if sec, ok := it.(*LayoutSection); ok {
			t.finalizeType(sec)
		}
	}

	// Обёртка всего тела в единственный <ac:layout>: свободный контент → single-секции.
	layout := &LayoutNode{}
	var loose []ast.Node
	flush := func() {
		if len(loose) == 0 {
			return
		}
		sec := &LayoutSection{SectionType: "single", Synthetic: true}
		cell := &LayoutCell{}
		for _, n := range loose {
			doc.RemoveChild(doc, n)
			cell.AppendChild(cell, n)
		}
		sec.AppendChild(sec, cell)
		layout.AppendChild(layout, sec)
		loose = nil
	}
	for _, it := range items {
		if sec, ok := it.(*LayoutSection); ok {
			flush()
			layout.AppendChild(layout, sec)
		} else {
			loose = append(loose, it)
		}
	}
	flush()
	doc.AppendChild(doc, layout)
}

// finalizeType выводит/валидирует ac:type секции по числу ячеек.
func (t *layoutTransformer) finalizeType(sec *LayoutSection) {
	cells := 0
	for c := sec.FirstChild(); c != nil; c = c.NextSibling() {
		if c.Kind() == KindLayoutCell {
			cells++
		}
	}
	if sec.SectionType == "" {
		if typ, err := inferLayoutType(cells); err != nil {
			t.collector.Add(err)
		} else {
			sec.SectionType = typ
		}
		return
	}
	want, ok := validLayoutTypes[sec.SectionType]
	if !ok {
		t.collector.Add(fmt.Errorf("layout: неизвестный type=%q", sec.SectionType))
	} else if cells != want {
		t.collector.Add(fmt.Errorf("layout: type=%s ожидает %d ячеек, найдено %d", sec.SectionType, want, cells))
	}
}

// ──────────────────────────── Рендереры ────────────────────────────

type layoutRenderer struct{}

func (r *layoutRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindLayout, r.renderLayout)
	reg.Register(KindLayoutSection, r.renderSection)
	reg.Register(KindLayoutCell, r.renderCell)
}

func (r *layoutRenderer) renderLayout(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<ac:layout>")
	} else {
		_, _ = w.WriteString("</ac:layout>")
	}
	return ast.WalkContinue, nil
}

func (r *layoutRenderer) renderSection(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		typ := n.(*LayoutSection).SectionType
		if typ == "" {
			typ = "single"
		}
		_, _ = fmt.Fprintf(w, `<ac:layout-section ac:type="%s">`, typ)
	} else {
		_, _ = w.WriteString("</ac:layout-section>")
	}
	return ast.WalkContinue, nil
}

func (r *layoutRenderer) renderCell(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<ac:layout-cell>")
	} else {
		_, _ = w.WriteString("</ac:layout-cell>")
	}
	return ast.WalkContinue, nil
}

// ──────────────────────────── Расширение ────────────────────────────

// LayoutExtension включает поддержку ":::"-раскладки. Collector обязателен —
// в него попадают ошибки валидации, которые нужно проверить после Convert.
type LayoutExtension struct{ Collector *LayoutCollector }

func (e *LayoutExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithBlockParsers(
			util.Prioritized(&layoutBlockParser{}, 100),
		),
		parser.WithASTTransformers(
			// Запускаем после spoiler (999), чтобы оборачивать уже готовое дерево.
			util.Prioritized(&layoutTransformer{collector: e.Collector}, 1000),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&layoutRenderer{}, 100),
		),
	)
}
