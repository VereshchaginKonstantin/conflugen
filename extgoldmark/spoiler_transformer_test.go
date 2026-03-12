package extgoldmark_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/VereshchaginKonstantin/conflugen/extgoldmark"
)

func TestSpoilerConverter_Convert(t *testing.T) {
	t.Parallel()

	t.Run("конвертация details в ui-expand", func(t *testing.T) {
		t.Parallel()

		// Arrange
		converter := extgoldmark.NewSpoilerConverter()
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
					&extgoldmark.SpoilerExtension{},
				),
			)
			var buf bytes.Buffer
			_ = md.Convert([]byte("# Test"), &buf)
		})
	})
}

func TestSpoilerBlock_Kind(t *testing.T) {
	t.Parallel()

	// Arrange
	block := &extgoldmark.SpoilerBlock{Summary: "test"}

	// Act
	kind := block.Kind()

	// Assert
	require.Equal(t, extgoldmark.KindSpoilerBlock, kind)
}

func TestSpoilerBlock_Dump(t *testing.T) {
	t.Parallel()

	// Arrange
	block := &extgoldmark.SpoilerBlock{Summary: "test summary"}

	// Act & Assert
	require.NotPanics(t, func() {
		block.Dump([]byte("source"), 0)
	})
}

func TestNewSpoilerExtension(t *testing.T) {
	t.Parallel()

	// Act
	ext := extgoldmark.NewSpoilerExtension()

	// Assert
	require.NotNil(t, ext)
}

func TestNewSpoilerASTTransformer(t *testing.T) {
	t.Parallel()

	// Act
	transformer := extgoldmark.NewSpoilerASTTransformer()

	// Assert
	require.NotNil(t, transformer)
}

func TestNewSpoilerRenderer(t *testing.T) {
	t.Parallel()

	// Act
	renderer := extgoldmark.NewSpoilerRenderer()

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
				&extgoldmark.SpoilerExtension{},
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
