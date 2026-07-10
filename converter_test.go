package main

import (
	"strings"
	"testing"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

func TestConvertMarkdown(t *testing.T) {
	t.Parallel()

	t.Run("конвертация простого markdown", func(t *testing.T) {
		t.Parallel()

		md := newMarkdownConverter(extensions.NewMermaidCollector(), extensions.NewLayoutCollector(), extensions.NewImageCollector())
		content := []byte("# Title\n\nParagraph text")

		html, hash, err := convertMarkdown(md, content)

		assertNoError(t, err)
		assertContains(t, html, "Title")
		assertContains(t, html, "Paragraph text")
		if len(hash) != 64 {
			t.Fatalf("expected 64 char hash, got %d", len(hash))
		}
	})

	t.Run("одинаковый контент — одинаковый хеш", func(t *testing.T) {
		t.Parallel()

		md := newMarkdownConverter(extensions.NewMermaidCollector(), extensions.NewLayoutCollector(), extensions.NewImageCollector())
		content := []byte("# Same")

		_, hash1, _ := convertMarkdown(md, content)
		_, hash2, _ := convertMarkdown(md, content)

		assertEqual(t, hash1, hash2)
	})

	t.Run("разный контент — разный хеш", func(t *testing.T) {
		t.Parallel()

		md := newMarkdownConverter(extensions.NewMermaidCollector(), extensions.NewLayoutCollector(), extensions.NewImageCollector())

		_, hash1, _ := convertMarkdown(md, []byte("# A"))
		_, hash2, _ := convertMarkdown(md, []byte("# B"))

		if hash1 == hash2 {
			t.Fatal("expected different hashes for different content")
		}
	})
}

func TestUnescapeConfluenceMacros(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "self-closing toc macro",
			input: `<p>&lt;ac:structured-macro ac:name=&quot;toc&quot; ac:schema-version=&quot;1&quot;/&gt;</p>`,
			want:  `<ac:structured-macro ac:name="toc" ac:schema-version="1"/>`,
		},
		{
			name:  "open+close toc macro",
			input: `<p>&lt;ac:structured-macro ac:name=&quot;toc&quot; ac:schema-version=&quot;1&quot;&gt;&lt;/ac:structured-macro&gt;</p>`,
			want:  `<ac:structured-macro ac:name="toc" ac:schema-version="1"></ac:structured-macro>`,
		},
		{
			name:  "no confluence macros",
			input: `<p>Hello <strong>world</strong></p>`,
			want:  `<p>Hello <strong>world</strong></p>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unescapeConfluenceMacros(tt.input)
			if strings.TrimSpace(got) != strings.TrimSpace(tt.want) {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestConvertMarkdownWithConfluenceMacro(t *testing.T) {
	t.Parallel()

	md := newMarkdownConverter(extensions.NewMermaidCollector(), extensions.NewLayoutCollector(), extensions.NewImageCollector())
	input := []byte("# Title\n\n<ac:structured-macro ac:name=\"toc\" ac:schema-version=\"1\"></ac:structured-macro>\n\n## Section\n\ntext\n")

	html, _, err := convertMarkdown(md, input)
	assertNoError(t, err)

	if !strings.Contains(html, `<ac:structured-macro ac:name="toc" ac:schema-version="1"></ac:structured-macro>`) {
		t.Errorf("confluence macro not found in output:\n%s", html)
	}
}

func TestConvertMarkdownWithLayout(t *testing.T) {
	t.Parallel()

	mermaid := extensions.NewMermaidCollector()
	layout := extensions.NewLayoutCollector()
	md := newMarkdownConverter(mermaid, layout, extensions.NewImageCollector())

	src := []byte("::: columns type=two_equal\n::: cell\nA\n:::\n::: cell\nB\n:::\n:::\n")
	htmlContent, _, err := convertMarkdown(md, src)
	assertNoError(t, err)
	if layout.Err() != nil {
		t.Fatalf("layout collector: %v", layout.Err())
	}
	for _, frag := range []string{
		`<ac:layout>`,
		`<ac:layout-section ac:type="two_equal">`,
		`<ac:layout-cell>`,
	} {
		if !strings.Contains(htmlContent, frag) {
			t.Fatalf("нет %q в выводе:\n%s", frag, htmlContent)
		}
	}
}

func TestConvertMarkdownLayoutError(t *testing.T) {
	t.Parallel()

	mermaid := extensions.NewMermaidCollector()
	layout := extensions.NewLayoutCollector()
	md := newMarkdownConverter(mermaid, layout, extensions.NewImageCollector())

	// '::: cell' без открытого '::: columns' — конвертация валидна, но коллектор ловит ошибку.
	src := []byte("::: cell\nA\n:::\n")
	_, _, err := convertMarkdown(md, src)
	assertNoError(t, err)
	if layout.Err() == nil {
		t.Fatal("ожидал ошибку в layout-коллекторе")
	}
}

func TestAnnotateHTML(t *testing.T) {
	t.Parallel()

	t.Run("добавляет хеш и подпись", func(t *testing.T) {
		t.Parallel()

		result := annotateHTML("<h1>Test</h1>", "abc123")

		assertContains(t, result, "<h1>Test</h1>")
		assertContains(t, result, "conflugen-auto-generated:abc123")
		assertContains(t, result, "conflugen")
	})
}
