package extgoldmark_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/VereshchaginKonstantin/conflugen/extgoldmark"
)

func TestPlantUMLExtension_Integration(t *testing.T) {
	t.Parallel()

	t.Run("plantuml блок конвертируется в макрос", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extgoldmark.PlantUMLExtension{},
			),
		)
		input := []byte("```plantuml\n@startuml\nAlice -> Bob: Hello\n@enduml\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, `ac:name="plantuml"`)
		require.Contains(t, output, "Alice -> Bob: Hello")
		require.Contains(t, output, "CDATA")
	})

	t.Run("обычный code block не затрагивается", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extgoldmark.PlantUMLExtension{},
			),
		)
		input := []byte("```go\nfmt.Println(\"hello\")\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.NotContains(t, output, `ac:name="plantuml"`)
	})

	t.Run("комбинация plantuml и code блоков", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extgoldmark.PlantUMLExtension{},
				&extgoldmark.ConfluenceCodeBlock{},
			),
		)
		input := []byte("```go\nfmt.Println(\"hello\")\n```\n\n```plantuml\n@startuml\nA -> B\n@enduml\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, `ac:name="plantuml"`)
		require.Contains(t, output, `ac:name="code"`)
	})

	t.Run("экранирование ]]> в содержимом", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extgoldmark.PlantUMLExtension{},
			),
		)
		input := []byte("```plantuml\n@startuml\nnote: data ]]> end\n@enduml\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, "]]&gt;")
	})

	t.Run("несколько plantuml блоков", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extgoldmark.PlantUMLExtension{},
			),
		)
		input := []byte("```plantuml\n@startuml\nA -> B\n@enduml\n```\n\n```plantuml\n@startuml\nC -> D\n@enduml\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Equal(t, 2, strings.Count(output, `ac:name="plantuml"`))
	})
}

func TestPlantUMLNode(t *testing.T) {
	t.Parallel()

	t.Run("Kind возвращает KindPlantUML", func(t *testing.T) {
		t.Parallel()

		// Arrange
		node := &extgoldmark.PlantUMLNode{}

		// Act
		kind := node.Kind()

		// Assert
		require.Equal(t, extgoldmark.KindPlantUML, kind)
	})

	t.Run("Dump не паникует", func(t *testing.T) {
		t.Parallel()

		// Arrange
		node := &extgoldmark.PlantUMLNode{}

		// Act & Assert
		require.NotPanics(t, func() {
			node.Dump([]byte("source"), 0)
		})
	})
}

func TestNewPlantUMLHTMLRenderer(t *testing.T) {
	t.Parallel()

	// Act
	renderer := extgoldmark.NewPlantUMLHTMLRenderer()

	// Assert
	require.NotNil(t, renderer)
}
