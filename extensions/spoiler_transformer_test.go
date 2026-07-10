package extensions_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

func TestSpoilerConverter_Convert(t *testing.T) {
	t.Parallel()

	t.Run("конвертация details в ui-expand", func(t *testing.T) {
		t.Parallel()

		// Arrange
		converter := extensions.NewSpoilerConverter()
		input := []byte(`<details>
<summary>Подробности</summary>

Содержимое спойлера

</details>`)

		// Act
		result, err := converter.Convert(input)

		// Assert
		require.NoError(t, err)
		output := string(result)
		require.Contains(t, output, "ui-expand")
	})
}

func TestSpoilerExtension_Integration(t *testing.T) {
	t.Parallel()

	t.Run("расширение регистрируется без паники", func(t *testing.T) {
		t.Parallel()

		// Act & Assert
		require.NotPanics(t, func() {
			md := goldmark.New(
				goldmark.WithExtensions(
					&extensions.SpoilerExtension{},
				),
			)
			var buf bytes.Buffer
			_ = md.Convert([]byte("# Test"), &buf)
		})
	})

	// Регрессия: AST-трансформер раньше ходил deep через ast.Walk и
	// reparent'ил вглубь — у параграфа уходил Text в spoiler, оставался <p />;
	// list items вываливались из <ul>; терялся <strong>. Теперь обходим только
	// top-level siblings документа между <details>/</details>.
	t.Run("multi-paragraph + list + bold сохраняет структуру", func(t *testing.T) {
		t.Parallel()
		src := "<details>\n<summary>title</summary>\n\nFirst paragraph.\n\nSecond with **bold** word.\n\n- one\n- two\n\n</details>\n"
		md := goldmark.New(
			goldmark.WithExtensions(&extensions.SpoilerExtension{}),
			goldmark.WithRendererOptions(html.WithUnsafe(), html.WithXHTML()),
		)
		var buf bytes.Buffer
		require.NoError(t, md.Convert([]byte(src), &buf))
		out := buf.String()

		require.Contains(t, out, `<ac:structured-macro ac:name="ui-expand"`, "должен быть ui-expand")
		require.Contains(t, out, `<p>First paragraph.</p>`, "первый <p> должен быть с текстом, не <p />")
		require.Contains(t, out, `<strong>bold</strong>`, "<strong> не должен пропасть")
		require.Contains(t, out, `<li>one</li>`, "<li>one должен быть внутри <ul>")
		require.Contains(t, out, `<li>two</li>`, "<li>two должен быть внутри <ul>")
		require.NotContains(t, out, "<p />", "пустых <p /> быть не должно")
		require.NotContains(t, out, "<p/>", "пустых <p/> быть не должно")
	})
}

func TestSpoilerBlock_Kind(t *testing.T) {
	t.Parallel()

	// Arrange
	block := &extensions.SpoilerBlock{Summary: "test"}

	// Act
	kind := block.Kind()

	// Assert
	require.Equal(t, extensions.KindSpoilerBlock, kind)
}

func TestSpoilerBlock_Dump(t *testing.T) {
	t.Parallel()

	// Arrange
	block := &extensions.SpoilerBlock{Summary: "test summary"}

	// Act & Assert
	require.NotPanics(t, func() {
		block.Dump([]byte("source"), 0)
	})
}

func TestNewSpoilerExtension(t *testing.T) {
	t.Parallel()

	// Act
	ext := extensions.NewSpoilerExtension()

	// Assert
	require.NotNil(t, ext)
}

func TestNewSpoilerASTTransformer(t *testing.T) {
	t.Parallel()

	// Act
	transformer := extensions.NewSpoilerASTTransformer()

	// Assert
	require.NotNil(t, transformer)
}

func TestNewSpoilerRenderer(t *testing.T) {
	t.Parallel()

	// Act
	renderer := extensions.NewSpoilerRenderer()

	// Assert
	require.NotNil(t, renderer)
}

func TestSpoilerRenderer_RenderSpoilerBlock(t *testing.T) {
	t.Parallel()

	t.Run("рендеринг спойлера с содержимым", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.SpoilerExtension{},
			),
		)

		input := []byte(`<details>
<summary>Показать</summary>

Текст внутри спойлера.

</details>`)

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.True(t,
			strings.Contains(output, "ui-expand") || strings.Contains(output, "<details"),
			"вывод должен содержать ui-expand или оставить details: %s", output,
		)
	})
}
