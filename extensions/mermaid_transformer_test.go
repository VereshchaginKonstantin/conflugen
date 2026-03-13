package extensions_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

func TestMermaidExtension_Integration(t *testing.T) {
	t.Parallel()

	t.Run("mermaid блок конвертируется в макрос", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.MermaidExtension{},
			),
		)
		input := []byte("```mermaid\ngraph TD\n    A[Запрос] --> B{Валидация}\n    B -->|OK| C[Обработка]\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, `ac:name="mermaid"`)
		require.Contains(t, output, "graph TD")
		require.Contains(t, output, "CDATA")
	})

	t.Run("обычный code block не затрагивается", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.MermaidExtension{},
			),
		)
		input := []byte("```go\nfmt.Println(\"hello\")\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.NotContains(t, output, `ac:name="mermaid"`)
	})

	t.Run("комбинация mermaid и code блоков", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.MermaidExtension{},
				&extensions.ConfluenceCodeBlock{},
			),
		)
		input := []byte("```go\nfmt.Println(\"hello\")\n```\n\n```mermaid\ngraph LR\n    A --> B\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, `ac:name="mermaid"`)
		require.Contains(t, output, `ac:name="code"`)
	})

	t.Run("экранирование ]]> в содержимом", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.MermaidExtension{},
			),
		)
		input := []byte("```mermaid\ngraph TD\n    A[\"data ]]> end\"] --> B\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, "]]&gt;")
	})

	t.Run("несколько mermaid блоков", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.MermaidExtension{},
			),
		)
		input := []byte("```mermaid\ngraph TD\n    A --> B\n```\n\n```mermaid\nsequenceDiagram\n    Alice->>Bob: Hello\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Equal(t, 2, strings.Count(output, `ac:name="mermaid"`))
	})

	t.Run("sequence diagram", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.MermaidExtension{},
			),
		)
		input := []byte("```mermaid\nsequenceDiagram\n    participant A as Клиент\n    participant B as Сервер\n    A->>B: Запрос\n    B-->>A: Ответ\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, `ac:name="mermaid"`)
		require.Contains(t, output, "sequenceDiagram")
	})
}

func TestMermaidNode(t *testing.T) {
	t.Parallel()

	t.Run("Kind возвращает KindMermaid", func(t *testing.T) {
		t.Parallel()

		// Arrange
		node := &extensions.MermaidNode{}

		// Act
		kind := node.Kind()

		// Assert
		require.Equal(t, extensions.KindMermaid, kind)
	})

	t.Run("Dump не паникует", func(t *testing.T) {
		t.Parallel()

		// Arrange
		node := &extensions.MermaidNode{}

		// Act & Assert
		require.NotPanics(t, func() {
			node.Dump([]byte("source"), 0)
		})
	})
}

func TestNewMermaidHTMLRenderer(t *testing.T) {
	t.Parallel()

	// Act
	renderer := extensions.NewMermaidHTMLRenderer()

	// Assert
	require.NotNil(t, renderer)
}
