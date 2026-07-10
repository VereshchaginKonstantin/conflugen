package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

// TestShowcaseProducesValidStorageXML — smoke-тест: прогоняет
// docs/examples/showcase.md через весь pipeline (include → macros →
// directive → apply → goldmark → unescape) и проверяет, что в выводе нет
// конструкций, на которых Confluence storage XHTML-парсер заведомо падает:
//   - <code><ac:…> (откр. тег макроса застрял внутри inline-code);
//   - <a href="ac:…"> (autolink-западня).
// Падал когда-то на «Unexpected close tag </code>; expected </ac:layout-section>»;
// этот тест ловит регрессию любого из двух классов багов.
func TestShowcaseProducesValidStorageXML(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("docs/examples/showcase.md")
	if err != nil {
		t.Skipf("showcase отсутствует: %v", err)
	}

	src, err = ExpandIncludes(filepath.Dir("docs/examples/showcase.md"), src, nil)
	if err != nil {
		t.Fatalf("ExpandIncludes: %v", err)
	}
	macros, src, err := ExtractMacros(src)
	if err != nil {
		t.Fatalf("ExtractMacros: %v", err)
	}
	macros, src, err = EnableStdlibPacks(src, macros)
	if err != nil {
		t.Fatalf("EnableStdlibPacks: %v", err)
	}
	_, cleaned, err := ParseDirective(src)
	if err != nil {
		t.Fatalf("ParseDirective: %v", err)
	}
	cleaned = ApplyMacros(cleaned, macros)
	cleaned = stripHTMLComments(cleaned)

	md := newMarkdownConverter(extensions.NewMermaidCollector(), extensions.NewLayoutCollector(), extensions.NewImageCollector())
	htmlContent, _, err := convertMarkdown(md, cleaned)
	if err != nil {
		t.Fatalf("convertMarkdown: %v", err)
	}

	for i, line := range strings.Split(htmlContent, "\n") {
		if strings.Contains(line, "<code><ac:") {
			t.Fatalf("строка %d содержит <code><ac:…> — макрос попал внутрь inline-code:\n%s", i+1, line)
		}
		if strings.Contains(line, `<a href="ac:`) {
			t.Fatalf("строка %d содержит <a href=\"ac:…\"> — autolink-западня:\n%s", i+1, line)
		}
		if strings.Contains(line, "<!-- raw HTML omitted -->") {
			t.Fatalf("строка %d содержит '<!-- raw HTML omitted -->' — goldmark выпил inline-HTML внутри макроса:\n%s", i+1, line)
		}
		if strings.Contains(line, "<!--") {
			t.Fatalf("строка %d содержит '<!--' — HTML-комментарий не вырезан, Confluence отвергает '--' в теле:\n%s", i+1, line)
		}
	}
}

func TestStripHTMLComments(t *testing.T) {
	t.Parallel()
	in := []byte(`# Doc
<!-- top-level comment with --double-dash inside -->
text
<!--
  multiline
  with --flag examples
-->
end`)
	out := stripHTMLComments(in)
	if strings.Contains(string(out), "<!--") {
		t.Fatalf("комментарии не вырезаны:\n%s", out)
	}
	if !strings.Contains(string(out), "# Doc") || !strings.Contains(string(out), "end") {
		t.Fatalf("обычный контент пропал:\n%s", out)
	}
}
