package extensions_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

func TestConfluenceLinkExtension_Integration(t *testing.T) {
	t.Parallel()

	t.Run("внешняя ссылка рендерится как <a>", func(t *testing.T) {
		t.Parallel()

		// Arrange
		ext := extensions.NewConfluenceLinkExtension("https://wiki.example.com", "OB", false)
		md := goldmark.New(
			goldmark.WithExtensions(ext),
			goldmark.WithParserOptions(parser.WithAutoHeadingID()),
			goldmark.WithRendererOptions(html.WithXHTML()),
		)
		input := []byte("[Google](https://google.com)")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, "https://google.com")
		require.Contains(t, output, "Google")
	})

	t.Run("email ссылка", func(t *testing.T) {
		t.Parallel()

		// Arrange
		ext := extensions.NewConfluenceLinkExtension("", "", false)
		md := goldmark.New(goldmark.WithExtensions(ext))
		input := []byte("[Contact](mailto:test@example.com)")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, "mailto:test@example.com")
	})

	t.Run("якорная ссылка", func(t *testing.T) {
		t.Parallel()

		// Arrange
		ext := extensions.NewConfluenceLinkExtension("", "", false)
		md := goldmark.New(goldmark.WithExtensions(ext))
		input := []byte("[Section](#section-1)")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, "#section-1")
	})

	t.Run("confluence страница как макрос", func(t *testing.T) {
		t.Parallel()

		// Arrange
		ext := extensions.NewConfluenceLinkExtension("https://confluence.example.com", "OB", true)
		md := goldmark.New(goldmark.WithExtensions(ext))
		input := []byte("[Page](https://confluence.example.com/pages/123)")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, "view-page")
	})

	t.Run("confluence страница без макроса", func(t *testing.T) {
		t.Parallel()

		// Arrange
		ext := extensions.NewConfluenceLinkExtension("https://confluence.example.com", "OB", false)
		md := goldmark.New(goldmark.WithExtensions(ext))
		input := []byte("[Page](https://confluence.example.com/pages/123)")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, "ac:link")
	})

	t.Run("несколько ссылок в документе", func(t *testing.T) {
		t.Parallel()

		// Arrange
		ext := extensions.NewConfluenceLinkExtension("", "", false)
		md := goldmark.New(goldmark.WithExtensions(ext))
		input := []byte("[A](https://a.com) and [B](https://b.com)")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, "https://a.com")
		require.Contains(t, output, "https://b.com")
	})
}

func TestConfluenceLinkNode_Kind(t *testing.T) {
	t.Parallel()

	// Arrange
	node := &extensions.ConfluenceLinkNode{}

	// Act
	kind := node.Kind()

	// Assert
	require.Equal(t, extensions.KindConfluenceLink, kind)
}

func TestConfluenceLinkNode_Dump(t *testing.T) {
	t.Parallel()

	// Arrange
	node := &extensions.ConfluenceLinkNode{}

	// Act & Assert
	require.NotPanics(t, func() {
		node.Dump([]byte("source"), 0)
	})
}

func TestNewConfluenceLinkRenderer(t *testing.T) {
	t.Parallel()

	// Act
	r := extensions.NewConfluenceLinkRenderer("https://wiki.example.com", "OB", true)

	// Assert
	require.NotNil(t, r)
}

func TestNewConfluenceLinkExtension(t *testing.T) {
	t.Parallel()

	// Act
	ext := extensions.NewConfluenceLinkExtension("https://wiki.example.com", "OB", false)

	// Assert
	require.NotNil(t, ext)
}

func TestAutoLinkExtension(t *testing.T) {
	t.Parallel()

	t.Run("регистрация расширения", func(t *testing.T) {
		t.Parallel()

		// Act & Assert
		require.NotPanics(t, func() {
			ext := extensions.NewAutoLinkExtension()
			md := goldmark.New(goldmark.WithExtensions(ext))
			var buf bytes.Buffer
			_ = md.Convert([]byte("Visit https://example.com today"), &buf)
		})
	})
}

func TestAutoLinkParser(t *testing.T) {
	t.Parallel()

	t.Run("создание парсера", func(t *testing.T) {
		t.Parallel()

		// Act
		p := extensions.NewAutoLinkParser()

		// Assert
		require.NotNil(t, p)
	})

	t.Run("триггерные символы", func(t *testing.T) {
		t.Parallel()

		// Arrange
		p := extensions.NewAutoLinkParser()

		// Act
		triggers := p.Trigger()

		// Assert
		require.Contains(t, triggers, byte('h'))
		require.Contains(t, triggers, byte('w'))
	})
}

func TestConfluenceLinkRenderer_RenderTypes(t *testing.T) {
	t.Parallel()

	t.Run("confluence пространство", func(t *testing.T) {
		t.Parallel()

		// Arrange
		ext := extensions.NewConfluenceLinkExtension("https://confluence.example.com", "OB", false)
		md := goldmark.New(goldmark.WithExtensions(ext))
		input := []byte("[Space](https://confluence.example.com/spaces/OB)")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, "ri:space")
	})

	t.Run("confluence вложение", func(t *testing.T) {
		t.Parallel()

		// Arrange
		ext := extensions.NewConfluenceLinkExtension("https://confluence.example.com", "OB", false)
		md := goldmark.New(goldmark.WithExtensions(ext))
		input := []byte("[File](https://confluence.example.com/attachments/file.pdf)")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, "ri:attachment")
	})

	t.Run("ссылка с title", func(t *testing.T) {
		t.Parallel()

		// Arrange
		ext := extensions.NewConfluenceLinkExtension("", "", false)
		md := goldmark.New(goldmark.WithExtensions(ext))
		input := []byte(`[Link](https://example.com "Title text")`)

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.True(t,
			strings.Contains(output, "Title text") || strings.Contains(output, "title"),
			"output should contain title: %s", output,
		)
	})
}
