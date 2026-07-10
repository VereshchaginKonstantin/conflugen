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
func newMarkdownConverter(
	mermaidCollector *extensions.MermaidCollector,
	layoutCollector *extensions.LayoutCollector,
	imageCollector *extensions.ImageCollector,
) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			&extensions.PlantUMLExtension{},
			&extensions.MermaidExtension{Collector: mermaidCollector},
			&extensions.ConfluenceCodeBlock{},
			&extensions.SpoilerExtension{},
			&extensions.LayoutExtension{Collector: layoutCollector},
			&extensions.ImageExtension{Collector: imageCollector},
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
			// WithUnsafe пропускает raw HTML в выводе. Это нужно, чтобы
			// внутренности шаблонов макросов вроде `<ac:rich-text-body>…<p>…</p>…</ac:rich-text-body>`
			// не превращались в «<!-- raw HTML omitted -->» (тогда Confluence
			// получает поломанный storage-XML). Источник markdown — наши же
			// файлы; security-риск тут не выше, чем у штатного `<ac:…>`
			// passthrough, который у нас и так есть через unescapeConfluenceMacros.
			html.WithUnsafe(),
		),
	)
}

// confluenceMacroRe находит экранированные Confluence макросы и ресурс-ссылки
// вида &lt;ac:structured-macro ...&gt;...&lt;/ac:structured-macro&gt; и
// self-closing &lt;ri:user ri:userkey=&quot;…&quot; /&gt;. Покрывает оба
// namespace'а: ac: (макросы) и ri: (resource identifier — user/page/space/
// attachment), включая атрибуты обоих namespace'ов.
var confluenceMacroRe = regexp.MustCompile(
	`(?s)&lt;(/?(?:ac|ri):[a-z-]+)((?:\s+(?:ac|ri):[a-z-]+=&quot;[^&]*&quot;)*)(\s*/?)&gt;`,
)

// namespacedAutolinkRe ловит ac:/ri: теги БЕЗ атрибутов, которые goldmark
// принял за autolink: `<ac:link>` выглядит как `<scheme:path>` и рендерится в
// `<a href="ac:link">ac:link</a>`. ac:/ri: — не реальные URL-схемы, по которым
// кто-то ставит ссылки, поэтому ложных срабатываний нет: возвращаем сырой тег.
// Закрывающий `</ac:link>` goldmark и так пропускает как raw inline HTML.
var namespacedAutolinkRe = regexp.MustCompile(
	`<a href="((?:ac|ri):[a-z-]+)"[^>]*>[^<]*</a>`,
)

// htmlCommentRe ловит любой <!-- … --> (многострочный). Confluence storage
// XHTML строго не разрешает `--` внутри тела комментария (только как часть
// закрывающего `-->`); поэтому совершенно безобидный markdown-комментарий
// вроде `<!-- usage: conflugen --url … --dry-run -->` валит публикацию
// 400-кой «String '--' not allowed in comment». Решение — выпилить ВСЕ
// HTML-комментарии из тела до goldmark: к этому моменту все наши директивы
// (+conflugen, +conflugen-include/-macro/-use) уже разобраны соответствующими
// шагами, остающиеся комментарии — это dev-заметки, которым на странице
// делать нечего.
var htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)

// stripHTMLComments — фильтр перед goldmark; ставится ПОСЛЕ ParseDirective +
// ExtractMacros + EnableStdlibPacks + ApplyMacros, чтобы директивы успели
// отработать.
func stripHTMLComments(content []byte) []byte {
	return htmlCommentRe.ReplaceAll(content, nil)
}

// inlineCodeRe защищает содержимое <code>…</code> от unescape — иначе
// `\`<ac:layout>\`` из markdown даст <code>&lt;ac:layout&gt;</code>,
// а unescape превратит это в <code><ac:layout></code> (открыт тег без
// закрытия). Confluence storage XHTML-парсер на таком падает.
// Аналогично — <pre>…</pre> для фенсированного кода без ConfluenceCodeBlock.
var inlineCodeRe = regexp.MustCompile(`(?s)<code>.*?</code>|<pre>.*?</pre>`)

// unescapeConfluenceMacros восстанавливает Confluence XML-макросы, экранированные goldmark.
// Goldmark не распознаёт XML namespace-теги (ac:structured-macro) как HTML
// и экранирует их в &lt;/&gt;. Эта функция возвращает их в исходный вид —
// но НЕ внутри <code>/<pre>, где такие последовательности — это намеренно
// показанная пользователем литеральная XML.
func unescapeConfluenceMacros(htmlStr string) string {
	// Шаг 0: вынимаем все <code>/<pre> блоки за плейсхолдеры — не трогаем их.
	var placeholders []string
	stash := inlineCodeRe.ReplaceAllStringFunc(htmlStr, func(match string) string {
		placeholders = append(placeholders, match)
		return fmt.Sprintf("\x00conflugen-code-%d\x00", len(placeholders)-1)
	})

	// Шаг 0.5: чиним ac:/ri: теги, которые goldmark превратил в autolink
	// (`<ac:link>` → `<a href="ac:link">ac:link</a>`). Делаем это ДО шага 1 и
	// на stash (где <code>/<pre> уже вынуты), чтобы не трогать литеральные
	// примеры в коде.
	stash = namespacedAutolinkRe.ReplaceAllString(stash, "<$1>")

	// Шаг 1: восстанавливаем экранированные ac:/ri: теги в остальном тексте.
	result := confluenceMacroRe.ReplaceAllStringFunc(stash, func(match string) string {
		match = strings.ReplaceAll(match, "&lt;", "<")
		match = strings.ReplaceAll(match, "&gt;", ">")
		match = strings.ReplaceAll(match, "&quot;", "\"")
		return match
	})

	// Шаг 2: убираем <p>...</p> обёртку у параграфов, содержащих ac: макросы.
	// Goldmark оборачивает любой блок, начинающийся с <ac:…> (которое он не
	// признал HTML-блоком из-за двоеточия в имени тега), в <p>…</p> —
	// Confluence storage-XHTML такого вокруг блочного макроса не принимает.
	//
	// Делать это regex'ом нельзя: если шаблон макроса сам содержит <p>…</p>
	// (как наши `box`-callout'ы), non-greedy `<p>.*?</p>` хватает вложенный
	// </p>, оставляя дисбаланс. Поэтому идём depth-aware сканером.
	result = unwrapParagraphsAroundMacros(result)

	// Шаг 3: возвращаем code/pre блоки на их места без изменений.
	for i, ph := range placeholders {
		result = strings.Replace(result, fmt.Sprintf("\x00conflugen-code-%d\x00", i), ph, 1)
	}

	return result
}

// unwrapParagraphsAroundMacros проходит html и для каждого <p>…</p>, чьё
// trim'нутое содержимое НАЧИНАЕТСЯ с `<ac:`, отбрасывает обрамляющие <p>/</p>.
// Это блочный макрос (ac:structured-macro/ac:layout/…), который Confluence
// не разрешает внутри <p>. Если же `<ac:…/>` сидит inline посреди текста
// (ac:emoticon, jira-link и т.п.) — оставляем <p>, потому что текст вокруг
// должен жить в параграфе. Закрывающий </p> ищем со счётчиком вложенности,
// чтобы не съесть внутренний </p> из шаблона box-callout'а.
func unwrapParagraphsAroundMacros(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		idx := strings.Index(s[i:], "<p>")
		if idx < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		b.WriteString(s[i : i+idx])
		open := i + idx
		inner := open + 3 // после "<p>"

		// Ищем закрывающий </p>, учитывая вложенные <p>.
		depth := 1
		j := inner
		for j < len(s) {
			if strings.HasPrefix(s[j:], "<p>") {
				depth++
				j += 3
				continue
			}
			if strings.HasPrefix(s[j:], "</p>") {
				depth--
				if depth == 0 {
					break
				}
				j += 4
				continue
			}
			j++
		}
		if depth != 0 {
			// Не нашли парный </p> — отдаём остаток как есть, дальше не идём
			// (некорректный HTML, но повреждать ещё больше — хуже).
			b.WriteString(s[open:])
			return b.String()
		}
		content := s[inner:j]
		i = j + 4 // после "</p>"

		trimmed := strings.TrimSpace(content)
		if strings.HasPrefix(trimmed, "<ac:") {
			b.WriteString(trimmed)
		} else {
			b.WriteString("<p>")
			b.WriteString(content)
			b.WriteString("</p>")
		}
	}
	return b.String()
}

// taskListCheckboxRe ловит чекбоксы GFM-task-list. Confluence storage-XHTML
// тихо выбрасывает <input>-формы при сохранении — пользователь видит
// «<li> текст</li>» без маркера. Заменяем на видимые Unicode-чекбоксы.
var taskListCheckboxRe = regexp.MustCompile(`<input\b[^>]*\btype="checkbox"[^>]*/?>`)

// transformTaskListCheckboxes конвертирует <input type="checkbox"/> →
// ☑ (checked) / ☐ (unchecked).
func transformTaskListCheckboxes(html string) string {
	return taskListCheckboxRe.ReplaceAllStringFunc(html, func(m string) string {
		if strings.Contains(m, "checked") {
			return "☑"
		}
		return "☐"
	})
}

// convertMarkdown конвертирует markdown контент (без директив) в Confluence HTML с хешем
func convertMarkdown(md goldmark.Markdown, content []byte) (string, string, error) {
	var buf strings.Builder
	if err := md.Convert(content, &buf); err != nil {
		return "", "", fmt.Errorf("convert markdown: %w", err)
	}

	// Порядок важен: сначала task-list → <ac:task-list> (для тех <ul>,
	// у которых ВСЕ дети — task-li); затем оставшиеся <input>-чекбоксы
	// (например, в смешанных списках, где не каждый <li> — задача) —
	// в Unicode ☑/☐. unescape — последним.
	htmlContent := unescapeConfluenceMacros(
		transformTaskListCheckboxes(
			transformTaskListsToConfluence(buf.String()),
		),
	)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(htmlContent)))

	return htmlContent, hash, nil
}

// annotateHTML добавляет подпись и хеш-макрос к HTML контенту
func annotateHTML(htmlContent, hash string) string {
	return htmlContent + "\n\n" +
		"<p>\n  <br/>\n</p>\n" +
		"<p><sub>conflugen-auto-generated:" + hash + "</sub></p>"
}
