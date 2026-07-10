package main

import (
	"strings"
	"testing"

	"github.com/yuin/goldmark"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

// newConv — конвертер с пустыми коллекторами, как в остальных тестах.
func newConv() goldmark.Markdown {
	return newMarkdownConverter(
		extensions.NewMermaidCollector(),
		extensions.NewLayoutCollector(),
		extensions.NewImageCollector(),
	)
}

// TestConvertMarkdownUserMention — сырое упоминание <ac:link><ri:user/></ac:link>
// inline внутри списка должно пройти насквозь, а не сломаться об autolink goldmark.
func TestConvertMarkdownUserMention(t *testing.T) {
	t.Parallel()

	md := newConv()
	src := []byte("- **Team Lead:** <ac:link><ri:user ri:userkey=\"ff8081818ea04ab1018f0af43d170527\" /></ac:link> — стратегия.\n")
	html, _, err := convertMarkdown(md, src)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	want := `<ac:link><ri:user ri:userkey="ff8081818ea04ab1018f0af43d170527" /></ac:link>`
	if !strings.Contains(html, want) {
		t.Fatalf("нет упоминания %q в выводе:\n%s", want, html)
	}
	for _, bad := range []string{`<a href="ac:link"`, `&lt;ri:user`, `&lt;ac:link`} {
		if strings.Contains(html, bad) {
			t.Fatalf("вывод содержит сломанный фрагмент %q:\n%s", bad, html)
		}
	}
}

// TestConvertMarkdownPageLinkRaw — сырая ссылка на страницу
// <ac:link><ri:page ri:content-title="..." /></ac:link> (с пробелами/эмодзи).
func TestConvertMarkdownPageLinkRaw(t *testing.T) {
	t.Parallel()

	md := newConv()
	src := []byte("- <ac:link><ri:page ri:content-title=\"💂 Дежурства B2C\" /></ac:link>\n")
	html, _, err := convertMarkdown(md, src)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	want := `<ac:link><ri:page ri:content-title="💂 Дежурства B2C" /></ac:link>`
	if !strings.Contains(html, want) {
		t.Fatalf("нет ссылки на страницу %q в выводе:\n%s", want, html)
	}
}

// TestUnescapeStillHandlesAcMacro — регресс: ac:-макрос по-прежнему
// восстанавливается (не сломали расширением регэкспа на ri:).
func TestUnescapeStillHandlesAcMacro(t *testing.T) {
	t.Parallel()

	got := unescapeConfluenceMacros(`&lt;ac:structured-macro ac:name=&quot;info&quot;&gt;x&lt;/ac:structured-macro&gt;`)
	want := `<ac:structured-macro ac:name="info">x</ac:structured-macro>`
	if !strings.Contains(got, want) {
		t.Fatalf("ac:-макрос сломан: %q", got)
	}
}
