package extgoldmark //nolint:testpackage // uses unexported functions

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizePageName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "удаление расширения .md",
			input:    "page.md",
			expected: "page",
		},
		{
			name:     "удаление расширения .MD",
			input:    "page.MD",
			expected: "page",
		},
		{
			name:     "удаление пути с /",
			input:    "subdir/page.md",
			expected: "page",
		},
		{
			name:     "замена пробелов на +",
			input:    "my page.md",
			expected: "my+page",
		},
		{
			name:     "замена _ на -",
			input:    "my_page.md",
			expected: "my-page",
		},
		{
			name:     "удаление спецсимволов",
			input:    "page?#%.md",
			expected: "page",
		},
		{
			name:     "без расширения",
			input:    "page",
			expected: "page",
		},
		{
			name:     "путь с обратным слешем",
			input:    "subdir\\page.md",
			expected: "page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			result := normalizePageName(tt.input)

			// Assert
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestEscapeXML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "амперсанд",
			input:    "a&b",
			expected: "a&amp;b",
		},
		{
			name:     "угловые скобки",
			input:    "<tag>",
			expected: "&lt;tag&gt;",
		},
		{
			name:     "кавычки",
			input:    `"value"`,
			expected: "&quot;value&quot;",
		},
		{
			name:     "апостроф",
			input:    "it's",
			expected: "it&#39;s",
		},
		{
			name:     "без спецсимволов",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "пустая строка",
			input:    "",
			expected: "",
		},
		{
			name:     "все спецсимволы",
			input:    `<a href="test&x='y'>`,
			expected: "&lt;a href=&quot;test&amp;x=&#39;y&#39;&gt;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			result := escapeXML(tt.input)

			// Assert
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineLinkType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		url      string
		baseURL  string
		expected string
	}{
		{
			name:     "email ссылка",
			url:      "mailto:test@example.com",
			expected: "email",
		},
		{
			name:     "якорь",
			url:      "#section",
			expected: "anchor",
		},
		{
			name:     "внешний URL",
			url:      "https://google.com",
			expected: "url",
		},
		{
			name:     "ссылка на .md файл",
			url:      "doc.md",
			expected: "url",
		},
		{
			name:     "относительный путь",
			url:      "subdir/page",
			expected: "url",
		},
		{
			name:     "confluence страница по пути /pages/",
			url:      "https://confluence.example.com/pages/123",
			expected: "page",
		},
		{
			name:     "confluence пространство",
			url:      "https://confluence.example.com/spaces/OB",
			expected: "space",
		},
		{
			name:     "confluence вложение",
			url:      "https://confluence.example.com/attachments/file.pdf",
			expected: "attachment",
		},
		{
			name:     "confluence общая ссылка",
			url:      "https://confluence.example.com/other",
			expected: "confluence",
		},
		{
			name:     "ссылка по baseURL с /display/",
			url:      "https://wiki.example.com/display/OB/Page",
			baseURL:  "https://wiki.example.com",
			expected: "page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			result := determineLinkType(tt.url, tt.baseURL)

			// Assert
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertToConfluenceLink(t *testing.T) {
	t.Parallel()

	t.Run("email ссылка", func(t *testing.T) {
		t.Parallel()

		// Act
		linkType, url := convertToConfluenceLink("mailto:test@example.com", "", "")

		// Assert
		require.Equal(t, "email", linkType)
		require.Equal(t, "mailto:test@example.com", url)
	})

	t.Run("якорная ссылка", func(t *testing.T) {
		t.Parallel()

		// Act
		linkType, url := convertToConfluenceLink("#section", "", "")

		// Assert
		require.Equal(t, "anchor", linkType)
		require.Equal(t, "#section", url)
	})

	t.Run("email ссылка сохраняется", func(t *testing.T) {
		t.Parallel()

		// Act
		linkType, resultURL := convertToConfluenceLink("mailto:test@example.com", "", "")

		// Assert
		require.Equal(t, "email", linkType)
		require.Equal(t, "mailto:test@example.com", resultURL)
	})

	t.Run("якорная ссылка сохраняется", func(t *testing.T) {
		t.Parallel()

		// Act
		linkType, resultURL := convertToConfluenceLink("#section", "", "")

		// Assert
		require.Equal(t, "anchor", linkType)
		require.Equal(t, "#section", resultURL)
	})

	t.Run("внешний URL остаётся как есть", func(t *testing.T) {
		t.Parallel()

		// Act
		linkType, resultURL := convertToConfluenceLink("https://google.com", "", "")

		// Assert
		require.Equal(t, "url", linkType)
		require.Equal(t, "https://google.com", resultURL)
	})

	t.Run("confluence страница возвращает page", func(t *testing.T) {
		t.Parallel()

		// Act
		linkType, resultURL := convertToConfluenceLink("https://confluence.example.com/pages/123", "", "")

		// Assert
		require.Equal(t, "page", linkType)
		require.Equal(t, "https://confluence.example.com/pages/123", resultURL)
	})
}

func TestExtractPageNameFromURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "URL с /display/ и именем страницы",
			url:      "https://wiki.example.com/display/OB/PageName",
			expected: "PageName",
		},
		{
			name:     "URL без /display/",
			url:      "https://wiki.example.com/pages/123",
			expected: "",
		},
		{
			name:     "URL с /display/ но без имени страницы",
			url:      "https://wiki.example.com/display/OB",
			expected: "",
		},
		{
			name:     "пустой URL",
			url:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			result := extractPageNameFromURL(tt.url)

			// Assert
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParseURL(t *testing.T) {
	t.Parallel()

	t.Run("полный URL", func(t *testing.T) {
		t.Parallel()

		// Act
		u, err := parseURL("https://example.com/path")

		// Assert
		require.NoError(t, err)
		require.Equal(t, "example.com", u.Host)
		require.Equal(t, "/path", u.Path)
	})

	t.Run("URL без схемы", func(t *testing.T) {
		t.Parallel()

		// Act
		u, err := parseURL("example.com/path")

		// Assert
		require.NoError(t, err)
		require.Equal(t, "example.com", u.Host)
	})

	t.Run("абсолютный путь", func(t *testing.T) {
		t.Parallel()

		// Act
		u, err := parseURL("/path/to/page")

		// Assert
		require.NoError(t, err)
		require.Equal(t, "/path/to/page", u.Path)
	})
}

func TestNewLinkTransformer(t *testing.T) {
	t.Parallel()

	// Act
	transformer := NewLinkTransformer("https://wiki.example.com/", "OB")

	// Assert
	require.Equal(t, "https://wiki.example.com", transformer.baseURL)
	require.Equal(t, "OB", transformer.confluenceSpace)
}
