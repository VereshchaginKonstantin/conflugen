package main

import (
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
