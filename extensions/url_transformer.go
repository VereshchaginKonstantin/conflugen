package extensions

import (
	"fmt"
	"net/url"
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

// Типы ссылок
const (
	linkTypeEmail      = "email"
	linkTypeAnchor     = "anchor"
	linkTypePage       = "page"
	linkTypeURL        = "url"
	linkTypeSpace      = "space"
	linkTypeAttachment = "attachment"
	linkTypeConfluence = "confluence"
)

// LinkTransformer преобразует ссылки Markdown в формат Confluence
type LinkTransformer struct {
	baseURL         string
	confluenceSpace string
}

// NewLinkTransformer создает новый трансформер ссылок
func NewLinkTransformer(
	baseURL, confluenceSpace string,
) *LinkTransformer {
	return &LinkTransformer{
		baseURL:         strings.TrimSuffix(baseURL, "/"),
		confluenceSpace: confluenceSpace,
	}
}

// Transform обрабатывает ссылки в документе
func (t *LinkTransformer) Transform(
	node *ast.Document, reader text.Reader, _ parser.Context,
) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		link, ok := n.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}

		confluenceLink := NewConfluenceLinkNode(
			link, reader, t.baseURL, t.confluenceSpace,
		)
		parent := n.Parent()
		if parent != nil {
			parent.InsertBefore(parent, n, confluenceLink)
			parent.RemoveChild(parent, n)
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})
}

// ConfluenceLinkNode кастомный узел для ссылок Confluence
type ConfluenceLinkNode struct {
	ast.BaseInline
	originalURL   string
	linkText      string
	linkType      string
	confluenceURL string
	title         string
}

// NewConfluenceLinkNode создает новый узел Confluence ссылки
func NewConfluenceLinkNode(
	link *ast.Link,
	reader text.Reader,
	baseURL, confluenceSpace string,
) *ConfluenceLinkNode {
	originalURL := string(link.Destination)
	textContent := extractLinkText(link, reader)

	if textContent == "" {
		textContent = originalURL
	}

	linkType, confluenceURL := convertToConfluenceLink(
		originalURL, baseURL, confluenceSpace,
	)

	title := ""
	if link.Title != nil {
		title = string(link.Title)
	}

	return &ConfluenceLinkNode{
		originalURL:   originalURL,
		linkText:      textContent,
		linkType:      linkType,
		confluenceURL: confluenceURL,
		title:         title,
	}
}

func extractLinkText(link *ast.Link, reader text.Reader) string {
	var linkText strings.Builder

	for child := link.FirstChild(); child != nil; child = child.NextSibling() {
		if textNode, ok := child.(*ast.Text); ok {
			linkText.Write(textNode.Value(reader.Source()))
			continue
		}

		em, ok := child.(*ast.Emphasis)
		if !ok {
			continue
		}

		for emChild := em.FirstChild(); emChild != nil; emChild = emChild.NextSibling() {
			if emText, ok := emChild.(*ast.Text); ok {
				linkText.Write(emText.Value(reader.Source()))
			}
		}
	}

	return linkText.String()
}

// determineLinkType определяет тип ссылки
func determineLinkType(urlStr, baseURL string) string {
	if strings.HasPrefix(urlStr, "mailto:") {
		return linkTypeEmail
	}

	if strings.HasPrefix(urlStr, "#") {
		return linkTypeAnchor
	}

	u, err := parseURL(urlStr)
	if err != nil {
		return determineLinkTypeFromRelative(urlStr)
	}

	return determineLinkTypeFromAbsolute(u, baseURL)
}

func determineLinkTypeFromRelative(urlStr string) string {
	if strings.HasSuffix(urlStr, ".md") ||
		strings.Contains(urlStr, "/") {
		return linkTypePage
	}
	return linkTypeURL
}

func determineLinkTypeFromAbsolute(
	u *url.URL, baseURL string,
) string {
	isConfluence := strings.Contains(u.Host, "confluence") ||
		(baseURL != "" && strings.HasPrefix(u.String(), baseURL))
	if !isConfluence {
		return linkTypeURL
	}

	path := u.Path
	switch {
	case strings.Contains(path, "/pages/"),
		strings.Contains(path, "/display/"):
		return linkTypePage
	case strings.Contains(path, "/spaces/"):
		return linkTypeSpace
	case strings.Contains(path, "/attachments/"),
		strings.Contains(path, "/download/"):
		return linkTypeAttachment
	default:
		return linkTypeConfluence
	}
}

// convertToConfluenceLink преобразует URL в формат Confluence
func convertToConfluenceLink(
	urlStr, baseURL, spaceKey string,
) (string, string) {
	linkType := determineLinkType(urlStr, baseURL)

	switch linkType {
	case linkTypeEmail, linkTypeAnchor:
		return linkType, urlStr
	case linkTypePage:
		return convertPageLink(urlStr, baseURL, spaceKey)
	case linkTypeAttachment:
		return convertAttachmentLink(urlStr, baseURL)
	default:
		return linkType, urlStr
	}
}

func convertPageLink(
	urlStr, baseURL, spaceKey string,
) (string, string) {
	if strings.HasSuffix(urlStr, ".md") {
		return buildPageURL(
			normalizePageName(urlStr), baseURL, spaceKey,
		)
	}

	if !strings.Contains(urlStr, "://") &&
		!strings.HasPrefix(urlStr, "/") {
		return buildPageURL(
			normalizePageName(urlStr), baseURL, spaceKey,
		)
	}

	return linkTypePage, urlStr
}

func buildPageURL(
	pageName, baseURL, spaceKey string,
) (string, string) {
	if spaceKey != "" && baseURL != "" {
		confluenceURL := fmt.Sprintf(
			"%s/display/%s/%s", baseURL, spaceKey, pageName,
		)
		return linkTypePage, confluenceURL
	}
	return linkTypePage, pageName
}

func convertAttachmentLink(
	urlStr, baseURL string,
) (string, string) {
	if !strings.Contains(urlStr, "://") && baseURL != "" {
		return linkTypeAttachment,
			baseURL + "/" + strings.TrimPrefix(urlStr, "/")
	}
	return linkTypeAttachment, urlStr
}

// normalizePageName нормализует имя страницы
func normalizePageName(name string) string {
	name = strings.TrimSuffix(name, ".md")
	name = strings.TrimSuffix(name, ".MD")

	if idx := strings.LastIndex(name, "/"); idx != -1 {
		name = name[idx+1:]
	}
	if idx := strings.LastIndex(name, "\\"); idx != -1 {
		name = name[idx+1:]
	}

	name = strings.ReplaceAll(name, " ", "+")
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, "?", "")
	name = strings.ReplaceAll(name, "#", "")
	name = strings.ReplaceAll(name, "%", "")

	return name
}

// parseURL парсит URL с обработкой ошибок
func parseURL(urlStr string) (*url.URL, error) {
	if !strings.Contains(urlStr, "://") &&
		!strings.HasPrefix(urlStr, "/") {
		urlStr = "http://" + urlStr
	}
	return url.Parse(urlStr)
}

// Dump для отладки
func (n *ConfluenceLinkNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"URL":   n.confluenceURL,
		"Text":  n.linkText,
		"Type":  n.linkType,
		"Title": n.title,
	}, nil)
}

// KindConfluenceLink тип узла Confluence ссылки
var KindConfluenceLink = ast.NewNodeKind("ConfluenceLink")

// Kind возвращает тип узла
func (n *ConfluenceLinkNode) Kind() ast.NodeKind {
	return KindConfluenceLink
}

// ConfluenceLinkRenderer рендерер для ссылок Confluence
type ConfluenceLinkRenderer struct {
	html.Config
	renderAsMacro bool
	baseURL       string
	spaceKey      string
}

// NewConfluenceLinkRenderer создает новый рендерер
func NewConfluenceLinkRenderer(
	baseURL, spaceKey string,
	renderAsMacro bool,
	opts ...html.Option,
) *ConfluenceLinkRenderer {
	r := &ConfluenceLinkRenderer{
		Config:        html.NewConfig(),
		baseURL:       baseURL,
		spaceKey:      spaceKey,
		renderAsMacro: renderAsMacro,
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

// RegisterFuncs регистрирует функцию рендеринга
func (r *ConfluenceLinkRenderer) RegisterFuncs(
	reg renderer.NodeRendererFuncRegisterer,
) {
	reg.Register(KindConfluenceLink, r.renderConfluenceLink)
}

// renderConfluenceLink рендерит ссылку Confluence
func (r *ConfluenceLinkRenderer) renderConfluenceLink(
	w util.BufWriter, _ []byte, node ast.Node, entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	linkNode, ok := node.(*ConfluenceLinkNode)
	if !ok {
		return ast.WalkContinue, nil
	}

	escapedText := escapeXML(linkNode.linkText)
	escapedURL := escapeXML(linkNode.confluenceURL)
	escapedTitle := escapeXML(linkNode.title)

	switch linkNode.linkType {
	case linkTypePage:
		r.renderPageLink(w, escapedURL, escapedText, escapedTitle)
	case linkTypeSpace:
		r.renderSpaceLink(w, escapedText)
	case linkTypeAttachment:
		r.renderAttachmentLink(w, escapedText)
	case linkTypeEmail:
		r.renderEmailLink(w, escapedURL, escapedText)
	case linkTypeAnchor:
		r.renderAnchorLink(w, escapedURL, escapedText)
	case linkTypeConfluence:
		r.renderConfluenceGenericLink(w, escapedURL, escapedText)
	default:
		r.renderExternalLink(
			w, escapedURL, escapedText, escapedTitle,
		)
	}

	return ast.WalkContinue, nil
}

// renderPageLink рендерит ссылку на страницу
func (r *ConfluenceLinkRenderer) renderPageLink(
	w util.BufWriter, linkURL, text, title string,
) {
	if !r.renderAsMacro {
		r.renderPageLinkStandard(w, text, title)
		return
	}

	r.renderPageLinkMacro(w, linkURL, text)
}

func (r *ConfluenceLinkRenderer) renderPageLinkMacro(
	w util.BufWriter, linkURL, text string,
) {
	_, _ = w.WriteString(
		`<ac:structured-macro ac:name="view-page" ` +
			`ac:schema-version="1">`,
	)

	pageName := text
	if strings.HasPrefix(linkURL, r.baseURL) && r.spaceKey != "" {
		if extracted := extractPageNameFromURL(linkURL); extracted != "" {
			pageName = extracted
		}
	}

	_, _ = fmt.Fprintf(w,
		`<ac:parameter ac:name="pageTitle">%s</ac:parameter>`,
		pageName,
	)
	_, _ = w.WriteString(`</ac:structured-macro>`)
}

func (r *ConfluenceLinkRenderer) renderPageLinkStandard(
	w util.BufWriter, text, title string,
) {
	_, _ = w.WriteString(`<ac:link>`)
	if title != "" {
		_, _ = fmt.Fprintf(w,
			`<ri:page ri:content-title="%s" ri:space-key="%s" />`,
			text, r.spaceKey,
		)
	} else {
		_, _ = fmt.Fprintf(w,
			`<ri:page ri:content-title="%s" />`, text,
		)
	}
	_, _ = w.WriteString(`</ac:link>`)
}

// renderSpaceLink рендерит ссылку на пространство
func (r *ConfluenceLinkRenderer) renderSpaceLink(
	w util.BufWriter, _ string,
) {
	_, _ = w.WriteString(`<ac:link><ri:space ri:space-key="`)
	_, _ = w.WriteString(r.spaceKey)
	_, _ = w.WriteString(`" /></ac:link>`)
}

// renderAttachmentLink рендерит ссылку на вложение
func (r *ConfluenceLinkRenderer) renderAttachmentLink(
	w util.BufWriter, text string,
) {
	_, _ = w.WriteString(`<ac:link><ri:attachment ri:filename="`)
	_, _ = w.WriteString(text)
	_, _ = w.WriteString(`" /></ac:link>`)
}

// renderEmailLink рендерит email ссылку
func (r *ConfluenceLinkRenderer) renderEmailLink(
	w util.BufWriter, linkURL, text string,
) {
	_, _ = fmt.Fprintf(w,
		`<a href="%s">%s</a>`, linkURL, text,
	)
}

// renderAnchorLink рендерит якорную ссылку
func (r *ConfluenceLinkRenderer) renderAnchorLink(
	w util.BufWriter, linkURL, text string,
) {
	_, _ = fmt.Fprintf(w,
		`<a href="%s">%s</a>`, linkURL, text,
	)
}

// renderConfluenceGenericLink рендерит общую ссылку Confluence
func (r *ConfluenceLinkRenderer) renderConfluenceGenericLink(
	w util.BufWriter, linkURL, text string,
) {
	_, _ = fmt.Fprintf(w,
		`<a href="%s">%s</a>`, linkURL, text,
	)
}

// renderExternalLink рендерит внешнюю ссылку
func (r *ConfluenceLinkRenderer) renderExternalLink(
	w util.BufWriter, linkURL, text, title string,
) {
	_, _ = fmt.Fprintf(w, `<a href="%s"`, linkURL)
	if title != "" {
		_, _ = fmt.Fprintf(w, ` title="%s"`, title)
	}
	_, _ = fmt.Fprintf(w, `>%s</a>`, text)
}

// extractPageNameFromURL извлекает имя страницы из URL
func extractPageNameFromURL(urlStr string) string {
	if idx := strings.Index(urlStr, "/display/"); idx != -1 {
		parts := strings.Split(urlStr[idx+9:], "/")
		if len(parts) >= 2 {
			return parts[1]
		}
	}
	return ""
}

// escapeXML экранирует XML/HTML символы
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// ConfluenceLinkExtension расширение для обработки ссылок
type ConfluenceLinkExtension struct {
	baseURL       string
	spaceKey      string
	renderAsMacro bool
}

// NewConfluenceLinkExtension создает новое расширение
func NewConfluenceLinkExtension(
	baseURL, spaceKey string, renderAsMacro bool,
) *ConfluenceLinkExtension {
	return &ConfluenceLinkExtension{
		baseURL:       baseURL,
		spaceKey:      spaceKey,
		renderAsMacro: renderAsMacro,
	}
}

// Extend расширяет Goldmark
func (e *ConfluenceLinkExtension) Extend(m goldmark.Markdown) {
	transformer := NewLinkTransformer(e.baseURL, e.spaceKey)

	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(transformer, 100),
		),
	)

	linkRenderer := NewConfluenceLinkRenderer(
		e.baseURL, e.spaceKey, e.renderAsMacro,
	)

	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(linkRenderer, 500),
		),
	)
}

// AutoLinkExtension для автоматического обнаружения URL в тексте
type AutoLinkExtension struct{}

// NewAutoLinkExtension создает расширение для авто-ссылок
func NewAutoLinkExtension() *AutoLinkExtension {
	return &AutoLinkExtension{}
}

// Extend добавляет авто-ссылки
func (e *AutoLinkExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithInlineParsers(
			util.Prioritized(NewAutoLinkParser(), 200),
		),
	)
}

// AutoLinkParser парсер авто-ссылок
type AutoLinkParser struct {
	urlRegex *regexp.Regexp
}

// NewAutoLinkParser создает парсер авто-ссылок
func NewAutoLinkParser() *AutoLinkParser {
	pattern := `(?:https?://|www\.)[\w\-]+(?:\.[\w\-]+)+` +
		`(?:[/?#][^\s<>"']*)?`
	return &AutoLinkParser{
		urlRegex: regexp.MustCompile(pattern),
	}
}

// Trigger возвращает триггерные символы
func (p *AutoLinkParser) Trigger() []byte {
	return []byte{'h', 'H', 'w', 'W', 'f', 'F', 'm', 'M'}
}

// Parse парсит авто-ссылки
func (p *AutoLinkParser) Parse(
	_ ast.Node, block text.Reader, _ parser.Context,
) ast.Node {
	line, segment := block.PeekLine()

	matches := p.urlRegex.FindIndex(line)
	if matches == nil || matches[0] != 0 {
		return nil
	}

	linkURL := string(line[matches[0]:matches[1]])
	link := ast.NewLink()

	if !strings.HasPrefix(linkURL, "http") &&
		strings.HasPrefix(linkURL, "www.") {
		link.Destination = []byte("http://" + linkURL)
	} else {
		link.Destination = []byte(linkURL)
	}

	link.AppendChild(
		link,
		ast.NewTextSegment(segment.WithStop(segment.Start+matches[1])),
	)
	block.Advance(matches[1])

	return link
}
