package main

import (
	"strings"
	"testing"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

// TestUnescapeIgnoresInlineCode: literal <ac:...> shown via backticks must stay
// escaped inside <code> — иначе получаем <code><ac:layout></code>, который
// Confluence storage XHTML-парсер отвергает.
func TestUnescapeIgnoresInlineCode(t *testing.T) {
	t.Parallel()
	input := `<p>see <code>&lt;ac:layout&gt;</code> for the wrapper</p>`
	got := unescapeConfluenceMacros(input)
	if !strings.Contains(got, `<code>&lt;ac:layout&gt;</code>`) {
		t.Fatalf("содержимое <code> должно остаться экранированным:\n%s", got)
	}
	if strings.Contains(got, `<code><ac:`) {
		t.Fatalf("в <code> просочился раскрытый <ac::\n%s", got)
	}
}

// TestUnescapeIgnoresInlinePre: то же для <pre>.
func TestUnescapeIgnoresInlinePre(t *testing.T) {
	t.Parallel()
	input := `<pre>&lt;ac:layout-section&gt;</pre>`
	got := unescapeConfluenceMacros(input)
	if !strings.Contains(got, `<pre>&lt;ac:layout-section&gt;</pre>`) {
		t.Fatalf("содержимое <pre> должно остаться:\n%s", got)
	}
}

// TestUnescapeStillTouchesOutsideCode: вне code/pre — старое поведение.
func TestUnescapeStillTouchesOutsideCode(t *testing.T) {
	t.Parallel()
	input := `<p>&lt;ac:structured-macro ac:name=&quot;toc&quot;/&gt;</p>`
	got := unescapeConfluenceMacros(input)
	if !strings.Contains(got, `<ac:structured-macro ac:name="toc"/>`) {
		t.Fatalf("ожидался unescape вне <code>:\n%s", got)
	}
}

// TestUnescapeShowcaseSafe: end-to-end — литеральный <ac:layout> в backticks
// + реальный ac:toc вне backticks → внутри code XML остаётся литеральной,
// настоящий toc-макрос проходит.
func TestUnescapeShowcaseSafe(t *testing.T) {
	t.Parallel()
	src := []byte("один: `<ac:layout>` в коде\n\n<ac:structured-macro ac:name=\"toc\"/>\n")
	md := newMarkdownConverter(extensions.NewMermaidCollector(), extensions.NewLayoutCollector(), extensions.NewImageCollector())
	htmlContent, _, err := convertMarkdown(md, src)
	assertNoError(t, err)
	if !strings.Contains(htmlContent, `<code>&lt;ac:layout&gt;</code>`) {
		t.Fatalf("<ac:layout> в backticks должно остаться escaped:\n%s", htmlContent)
	}
	if !strings.Contains(htmlContent, `<ac:structured-macro ac:name="toc"/>`) {
		t.Fatalf("настоящий ac:toc должен пройти:\n%s", htmlContent)
	}
}
