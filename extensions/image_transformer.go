package extensions

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
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

// ImageAttachment — локальный файл-изображение, который нужно залить на
// страницу как attachment до публикации.
type ImageAttachment struct {
	Filename string // имя файла на странице Confluence (basename)
	Path     string // абсолютный путь на диске для чтения содержимого
}

// ImageCollector аккумулирует относительные локальные изображения,
// встреченные при рендере markdown'а одной страницы. baseDir задаётся через
// Reset перед каждым файлом и используется для резолва относительных src.
type ImageCollector struct {
	mu      sync.Mutex
	baseDir string
	images  []ImageAttachment
	seen    map[string]struct{}
}

// NewImageCollector создаёт пустой коллектор.
func NewImageCollector() *ImageCollector {
	return &ImageCollector{seen: map[string]struct{}{}}
}

// Reset очищает коллектор и задаёт baseDir для резолва относительных путей.
// Вызывается перед конвертацией каждого .md файла.
func (c *ImageCollector) Reset(baseDir string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.baseDir = baseDir
	c.images = c.images[:0]
	c.seen = map[string]struct{}{}
}

// BaseDir возвращает текущую базовую директорию.
func (c *ImageCollector) BaseDir() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseDir
}

// resolve проверяет, что src — относительный путь к существующему файлу,
// и возвращает (basename, absPath, true). Если src внешний (http/https/...)
// или файла нет — (..., false). Логирует warning для «должен бы быть файл, а его нет».
func (c *ImageCollector) resolve(src string) (string, string, bool) {
	c.mu.Lock()
	baseDir := c.baseDir
	c.mu.Unlock()

	if src == "" {
		return "", "", false
	}

	// Внешние URL не трогаем — Confluence storage format спокойно принимает
	// <img src="https://…">, отдельный аплоад не нужен.
	if isExternalURL(src) {
		return "", "", false
	}

	// markdown-ссылки часто URL-кодированы (пробелы как %20). Резолвим
	// относительно файла с +conflugen-директивой; querystring/fragment
	// для локальных файлов отрезаем.
	cleaned := src
	if i := strings.IndexAny(cleaned, "?#"); i != -1 {
		cleaned = cleaned[:i]
	}
	if decoded, err := url.PathUnescape(cleaned); err == nil {
		cleaned = decoded
	}

	if baseDir == "" {
		// Без baseDir не имеем права резолвить — оставим как есть, упадёт в дефолт.
		return "", "", false
	}

	abs := cleaned
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(baseDir, cleaned)
	}
	abs = filepath.Clean(abs)

	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		log.Printf("предупреждение: изображение не найдено, пропускаю: %s (искал %s)", src, abs)
		return "", "", false
	}

	filename := filepath.Base(abs)
	return filename, abs, true
}

// add регистрирует найденный файл в коллекторе (идемпотентно по filename).
func (c *ImageCollector) add(filename, absPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.seen[filename]; ok {
		return
	}
	c.seen[filename] = struct{}{}
	c.images = append(c.images, ImageAttachment{Filename: filename, Path: absPath})
}

// Images возвращает копию списка собранных изображений.
func (c *ImageCollector) Images() []ImageAttachment {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]ImageAttachment, len(c.images))
	copy(out, c.images)
	return out
}

// extractInlineText собирает текст всех inline-детей узла. Используется как
// замена deprecated ast.Node.Text — для ast.Image дети это Text-сегменты с
// alt-описанием.
func extractInlineText(n ast.Node, source []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(source))
		case *ast.String:
			b.Write(t.Value)
		case *ast.RawHTML:
			for i := 0; i < t.Segments.Len(); i++ {
				seg := t.Segments.At(i)
				b.Write(seg.Value(source))
			}
		case *ast.AutoLink:
			b.Write(t.URL(source))
		default:
			b.WriteString(extractInlineText(c, source))
		}
	}
	return b.String()
}

func isExternalURL(src string) bool {
	lower := strings.ToLower(src)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "//")
}

// ──────────────────────────── Узел ────────────────────────────

// KindConfluenceImage — тип узла «локальное изображение, заливаемое как attachment».
var KindConfluenceImage = ast.NewNodeKind("ConfluenceImage")

// ConfluenceImageNode хранит уже зарезолвленное имя attachment'а и alt-текст.
// Сам путь на диске не хранится — он живёт в коллекторе.
type ConfluenceImageNode struct {
	ast.BaseInline
	Filename string
	Alt      string
}

// Kind возвращает тип узла.
func (n *ConfluenceImageNode) Kind() ast.NodeKind { return KindConfluenceImage }

// Dump для отладки.
func (n *ConfluenceImageNode) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{
		"Filename": n.Filename,
		"Alt":      n.Alt,
	}, nil)
}

// ──────────────────────────── Трансформер ────────────────────────────

// ImageTransformer обходит AST, находит ast.Image с относительным src,
// проверяет существование файла и заменяет узел на ConfluenceImageNode.
// Внешние URL и несуществующие файлы остаются дефолтными ast.Image —
// goldmark/html отрендерит их как обычный <img>.
type ImageTransformer struct {
	Collector *ImageCollector
}

// Transform — реализация parser.ASTTransformer.
func (t *ImageTransformer) Transform(
	node *ast.Document, reader text.Reader, _ parser.Context,
) {
	if t.Collector == nil {
		return
	}

	type replacement struct {
		parent ast.Node
		old    ast.Node
		new    ast.Node
	}
	var replacements []replacement

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		img, ok := n.(*ast.Image)
		if !ok {
			return ast.WalkContinue, nil
		}

		src := string(img.Destination)
		filename, absPath, ok := t.Collector.resolve(src)
		if !ok {
			return ast.WalkContinue, nil
		}

		t.Collector.add(filename, absPath)

		alt := extractInlineText(img, reader.Source())
		replacements = append(replacements, replacement{
			parent: n.Parent(),
			old:    n,
			new:    &ConfluenceImageNode{Filename: filename, Alt: alt},
		})
		return ast.WalkContinue, nil
	})

	for _, r := range replacements {
		if r.parent == nil {
			continue
		}
		r.parent.InsertBefore(r.parent, r.old, r.new)
		r.parent.RemoveChild(r.parent, r.old)
	}
}

// ──────────────────────────── Рендерер ────────────────────────────

// ImageRenderer рендерит ConfluenceImageNode в <ac:image><ri:attachment .../></ac:image>.
type ImageRenderer struct {
	html.Config
}

// NewImageRenderer создаёт рендерер.
func NewImageRenderer(opts ...html.Option) renderer.NodeRenderer {
	r := &ImageRenderer{Config: html.NewConfig()}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

// RegisterFuncs регистрирует функцию рендеринга для KindConfluenceImage.
func (r *ImageRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindConfluenceImage, r.renderImage)
}

func (r *ImageRenderer) renderImage(
	w util.BufWriter, _ []byte, node ast.Node, entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n, ok := node.(*ConfluenceImageNode)
	if !ok {
		return ast.WalkContinue, nil
	}

	_, _ = w.WriteString(`<ac:image`)
	if n.Alt != "" {
		_, _ = fmt.Fprintf(w, ` ac:alt="%s"`, escapeXMLAttr(n.Alt))
	}
	_, _ = fmt.Fprintf(w,
		`><ri:attachment ri:filename="%s" /></ac:image>`,
		escapeXMLAttr(n.Filename),
	)
	return ast.WalkSkipChildren, nil
}

// escapeXMLAttr экранирует символы, недопустимые в значении XML-атрибута.
func escapeXMLAttr(s string) string {
	r := strings.NewReplacer(
		`&`, `&amp;`,
		`<`, `&lt;`,
		`>`, `&gt;`,
		`"`, `&quot;`,
		`'`, `&apos;`,
	)
	return r.Replace(s)
}

// ──────────────────────────── Расширение ────────────────────────────

// ImageExtension подключает image-трансформер и рендерер к goldmark.
type ImageExtension struct {
	Collector *ImageCollector
}

// Extend — реализация goldmark.Extender.
func (e *ImageExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(&ImageTransformer{Collector: e.Collector}, 100),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(NewImageRenderer(), 500),
		),
	)
}
