package extensions_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

func newImageMD(baseDir string) (goldmark.Markdown, *extensions.ImageCollector) {
	collector := extensions.NewImageCollector()
	collector.Reset(baseDir)
	md := goldmark.New(
		goldmark.WithExtensions(
			&extensions.ImageExtension{Collector: collector},
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
			html.WithXHTML(),
		),
	)
	return md, collector
}

func writeFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("fake-image-bytes"), 0o644))
	return path
}

func TestImageExtension_RelativePath(t *testing.T) {
	t.Parallel()

	t.Run("относительный путь к существующему файлу → ac:image+ri:attachment", func(t *testing.T) {
		t.Parallel()

		// Arrange
		dir := t.TempDir()
		writeFile(t, dir, "pic.png")
		md, collector := newImageMD(dir)
		input := []byte("![architecture](pic.png)")

		// Act
		var buf bytes.Buffer
		require.NoError(t, md.Convert(input, &buf))

		// Assert
		out := buf.String()
		require.Contains(t, out, `<ac:image`)
		require.Contains(t, out, `ac:alt="architecture"`)
		require.Contains(t, out, `<ri:attachment ri:filename="pic.png" />`)
		require.NotContains(t, out, `<img`)

		imgs := collector.Images()
		require.Len(t, imgs, 1)
		require.Equal(t, "pic.png", imgs[0].Filename)
		require.Equal(t, filepath.Join(dir, "pic.png"), imgs[0].Path)
	})

	t.Run("относительный путь в подкаталог", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, dir, "diagrams/flow.png")
		md, collector := newImageMD(dir)
		input := []byte("![flow](diagrams/flow.png)")

		var buf bytes.Buffer
		require.NoError(t, md.Convert(input, &buf))

		out := buf.String()
		require.Contains(t, out, `<ri:attachment ri:filename="flow.png" />`)

		imgs := collector.Images()
		require.Len(t, imgs, 1)
		require.Equal(t, "flow.png", imgs[0].Filename)
	})

	t.Run("URL-кодированный путь с пробелом", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, dir, "my pic.png")
		md, collector := newImageMD(dir)
		input := []byte("![p](my%20pic.png)")

		var buf bytes.Buffer
		require.NoError(t, md.Convert(input, &buf))

		out := buf.String()
		require.Contains(t, out, `<ri:attachment ri:filename="my pic.png" />`)
		require.Len(t, collector.Images(), 1)
	})
}

func TestImageExtension_MissingFile(t *testing.T) {
	t.Parallel()

	t.Run("несуществующий файл → default <img>, ничего не аплоадим", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		md, collector := newImageMD(dir)
		input := []byte("![missing](nope.png)")

		var buf bytes.Buffer
		require.NoError(t, md.Convert(input, &buf))

		out := buf.String()
		require.Contains(t, out, `<img`)
		require.Contains(t, out, `src="nope.png"`)
		require.NotContains(t, out, `<ac:image`)
		require.Empty(t, collector.Images())
	})
}

func TestImageExtension_ExternalURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, src string
	}{
		{"https", "https://example.com/pic.png"},
		{"http", "http://example.com/pic.png"},
		{"protocol-relative", "//example.com/pic.png"},
		{"data url", "data:image/png;base64,AAA"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			md, collector := newImageMD(dir)
			input := []byte("![ext](" + tc.src + ")")

			var buf bytes.Buffer
			require.NoError(t, md.Convert(input, &buf))

			out := buf.String()
			require.Contains(t, out, `<img`)
			require.NotContains(t, out, `<ac:image`)
			require.Empty(t, collector.Images())
		})
	}
}

func TestImageExtension_Dedup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "pic.png")
	md, collector := newImageMD(dir)
	input := []byte("![one](pic.png)\n\n![two](pic.png)")

	var buf bytes.Buffer
	require.NoError(t, md.Convert(input, &buf))

	// Оба вхождения должны отрендериться, но в коллекторе — один файл.
	imgs := collector.Images()
	require.Len(t, imgs, 1)
}

func TestImageExtension_Reset(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "pic.png")
	md, collector := newImageMD(dir)
	var buf bytes.Buffer
	require.NoError(t, md.Convert([]byte("![p](pic.png)"), &buf))
	require.Len(t, collector.Images(), 1)

	collector.Reset(t.TempDir())
	require.Empty(t, collector.Images())
}

func TestImageExtension_NoBaseDir(t *testing.T) {
	t.Parallel()

	// Без Reset(baseDir) — относительный путь не резолвим, дефолт <img>.
	collector := extensions.NewImageCollector()
	md := goldmark.New(
		goldmark.WithExtensions(
			&extensions.ImageExtension{Collector: collector},
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	var buf bytes.Buffer
	require.NoError(t, md.Convert([]byte("![p](pic.png)"), &buf))
	require.NotContains(t, buf.String(), `<ac:image`)
	require.Empty(t, collector.Images())
}

func TestImageExtension_AltEscaping(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "p.png")
	md, _ := newImageMD(dir)
	input := []byte(`![A & B "quoted" <tag>](p.png)`)

	var buf bytes.Buffer
	require.NoError(t, md.Convert(input, &buf))

	out := buf.String()
	require.Contains(t, out, `ac:alt="A &amp; B &quot;quoted&quot; &lt;tag&gt;"`)
}
