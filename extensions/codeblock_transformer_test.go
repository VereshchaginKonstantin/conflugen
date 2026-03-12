package extensions_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

func TestConfluenceCodeBlock_Integration(t *testing.T) {
	t.Parallel()

	t.Run("fenced code block с языком", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.ConfluenceCodeBlock{},
			),
		)
		input := []byte("```go\nfmt.Println(\"hello\")\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, `ac:name="code"`)
		require.Contains(t, output, `ac:name="language"`)
		require.Contains(t, output, "go")
		require.Contains(t, output, "CDATA")
	})

	t.Run("fenced code block без языка", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.ConfluenceCodeBlock{},
			),
		)
		input := []byte("```\nsome code\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Contains(t, output, `ac:name="code"`)
		require.NotContains(t, output, `ac:name="language"`)
	})

	t.Run("plantuml блок пропускается", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.ConfluenceCodeBlock{},
			),
		)
		input := []byte("```plantuml\n@startuml\nA -> B\n@enduml\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		// plantuml блоки не должны рендериться как code
		require.NotContains(t, output, `ac:name="language">plantuml`,
			"plantuml блок не должен рендериться как code block",
		)
	})

	t.Run("puml блок пропускается", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.ConfluenceCodeBlock{},
			),
		)
		input := []byte("```puml\n@startuml\nA -> B\n@enduml\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.NotContains(t, output, `ac:name="language">puml`,
			"puml блок не должен рендериться как code block",
		)
	})

	t.Run("несколько code блоков", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.ConfluenceCodeBlock{},
			),
		)
		input := []byte("```python\nprint('hello')\n```\n\n```sql\nSELECT 1\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		output := buf.String()
		require.Equal(t, 2, strings.Count(output, `ac:name="code"`))
	})

	t.Run("HTML экранирование языка", func(t *testing.T) {
		t.Parallel()

		// Arrange
		md := goldmark.New(
			goldmark.WithExtensions(
				&extensions.ConfluenceCodeBlock{},
			),
		)
		input := []byte("```c++\nint main() {}\n```")

		// Act
		var buf bytes.Buffer
		err := md.Convert(input, &buf)

		// Assert
		require.NoError(t, err)
		// Просто проверяем что не крашится
	})
}

func TestNew(t *testing.T) {
	t.Parallel()

	// Act
	ext := extensions.New()

	// Assert
	require.NotNil(t, ext)
	require.IsType(t, &extensions.ConfluenceCodeBlock{}, ext)
}
