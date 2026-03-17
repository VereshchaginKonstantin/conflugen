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

		md := newMarkdownConverter(extensions.NewMermaidCollector())
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

		md := newMarkdownConverter(extensions.NewMermaidCollector())
		content := []byte("# Same")

		_, hash1, _ := convertMarkdown(md, content)
		_, hash2, _ := convertMarkdown(md, content)

		assertEqual(t, hash1, hash2)
	})

	t.Run("разный контент — разный хеш", func(t *testing.T) {
		t.Parallel()

		md := newMarkdownConverter(extensions.NewMermaidCollector())

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

	md := newMarkdownConverter(extensions.NewMermaidCollector())
	input := []byte("# Title\n\n<ac:structured-macro ac:name=\"toc\" ac:schema-version=\"1\"></ac:structured-macro>\n\n## Section\n\ntext\n")

	html, _, err := convertMarkdown(md, input)
	assertNoError(t, err)

	if !strings.Contains(html, `<ac:structured-macro ac:name="toc" ac:schema-version="1"></ac:structured-macro>`) {
		t.Errorf("confluence macro not found in output:\n%s", html)
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
