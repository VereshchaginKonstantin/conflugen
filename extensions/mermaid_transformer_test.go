package extensions_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

func newMermaidMD() (goldmark.Markdown, *extensions.MermaidCollector) {
	collector := extensions.NewMermaidCollector()
	md := goldmark.New(
		goldmark.WithExtensions(
			&extensions.MermaidExtension{Collector: collector},
		),
	)
	return md, collector
}

func TestMermaidExtension_Integration(t *testing.T) {
	t.Parallel()

	t.Run("mermaid блок конвертируется в макрос mermaid-cloud", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md, _ := newMermaidMD()
		input := []byte("```mermaid\ngraph TD\n    A[Запрос] --> B{Валидация}\n    B -->|OK| C[Обработка]\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, `ac:name="mermaid-cloud"`)
		require.Contains(t, output, `ac:name="filename"`)
		require.Contains(t, output, `ac:name="format">svg`)
	})

	t.Run("collector собирает диаграммы", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md, collector := newMermaidMD()
		input := []byte("```mermaid\ngraph TD\n    A --> B\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		diagrams := collector.Diagrams()
		require.Len(t, diagrams, 1)
		require.Contains(t, diagrams[0].Content, "graph TD")
		require.NotEmpty(t, diagrams[0].Filename)
	})

	t.Run("обычный code block не затрагивается", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md, collector := newMermaidMD()
		input := []byte("```go\nfmt.Println(\"hello\")\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.NotContains(t, output, `ac:name="mermaid-cloud"`)
		require.Empty(t, collector.Diagrams())
	})

	t.Run("комбинация mermaid и code блоков", func(t *testing.T) {
		t.Parallel()

		// Arrange
		collector := extensions.NewMermaidCollector()
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.MermaidExtension{Collector: collector},
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
		require.Contains(t, output, `ac:name="mermaid-cloud"`)
		require.Contains(t, output, `ac:name="code"`)
	})

	t.Run("несколько mermaid блоков", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md, collector := newMermaidMD()
		input := []byte("```mermaid\ngraph TD\n    A --> B\n```\n\n```mermaid\nsequenceDiagram\n    Alice->>Bob: Hello\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Equal(t, 2, strings.Count(output, `ac:name="mermaid-cloud"`))
		require.Len(t, collector.Diagrams(), 2)
	})

	t.Run("sequence diagram", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md, collector := newMermaidMD()
		input := []byte("```mermaid\nsequenceDiagram\n    participant A as Клиент\n    participant B as Сервер\n    A->>B: Запрос\n    B-->>A: Ответ\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, `ac:name="mermaid-cloud"`)
		diagrams := collector.Diagrams()
		require.Len(t, diagrams, 1)
		require.Contains(t, diagrams[0].Content, "sequenceDiagram")
	})

	t.Run("filename основан на хеше контента", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md, collector := newMermaidMD()
		input := []byte("```mermaid\ngraph TD\n    A --> B\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		diagrams := collector.Diagrams()
		require.True(t, strings.HasPrefix(diagrams[0].Filename, "mermaid-"))
		require.Len(t, diagrams[0].Filename, len("mermaid-")+8)
	})

	t.Run("reset очищает коллектор", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md, collector := newMermaidMD()
		input := []byte("```mermaid\ngraph TD\n    A --> B\n```")

		var buf bytes.Buffer
		_ = md.Convert(input, &buf)
		require.Len(t, collector.Diagrams(), 1)

		// Act
		collector.Reset()

		// Assert
		require.Empty(t, collector.Diagrams())
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
	collector := extensions.NewMermaidCollector()
	renderer := extensions.NewMermaidHTMLRenderer(collector)

	// Assert
	require.NotNil(t, renderer)
}
