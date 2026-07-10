package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeFile %s: %v", name, err)
	}
	return path
}

func TestExpandIncludesSimple(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "header.md", "## Заголовок\n")
	src := []byte("# Doc\n\n<!-- +conflugen-include header.md -->\n\ntext\n")

	out, err := ExpandIncludes(dir, src, nil)
	assertNoError(t, err)

	got := string(out)
	if !strings.Contains(got, "## Заголовок") {
		t.Fatalf("ожидался заголовок из header.md, got:\n%s", got)
	}
	if strings.Contains(got, "+conflugen-include") {
		t.Fatalf("директива не была заменена:\n%s", got)
	}
	if !strings.Contains(got, "# Doc") || !strings.Contains(got, "text") {
		t.Fatalf("окружение потерялось:\n%s", got)
	}
}

func TestExpandIncludesNested(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sub, "inner.md", "INNER\n")
	writeFile(t, dir, "middle.md", "MID before\n<!-- +conflugen-include sub/inner.md -->\nMID after\n")
	src := []byte("TOP\n<!-- +conflugen-include middle.md -->\nEND\n")

	out, err := ExpandIncludes(dir, src, nil)
	assertNoError(t, err)

	got := string(out)
	for _, want := range []string{"TOP", "MID before", "INNER", "MID after", "END"} {
		if !strings.Contains(got, want) {
			t.Fatalf("нет фрагмента %q в:\n%s", want, got)
		}
	}
	if strings.Contains(got, "+conflugen-include") {
		t.Fatalf("осталась нераскрытая директива:\n%s", got)
	}
}

func TestExpandIncludesRelativeToIncludingFile(t *testing.T) {
	t.Parallel()

	// dir/a.md → подключает sub/b.md, который сам подключает c.md
	// c.md лежит рядом с b.md (в sub/), не в dir/.
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sub, "c.md", "C-CONTENT\n")
	writeFile(t, sub, "b.md", "B<<<\n<!-- +conflugen-include c.md -->\nB>>>\n")

	src := []byte("A\n<!-- +conflugen-include sub/b.md -->\nA-END\n")

	out, err := ExpandIncludes(dir, src, nil)
	assertNoError(t, err)
	if !strings.Contains(string(out), "C-CONTENT") {
		t.Fatalf("c.md не подключился (должен резолвиться относительно b.md):\n%s", string(out))
	}
}

func TestExpandIncludesCycle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "a.md", "A\n<!-- +conflugen-include b.md -->\n")
	writeFile(t, dir, "b.md", "B\n<!-- +conflugen-include a.md -->\n")

	src := []byte("<!-- +conflugen-include a.md -->\n")
	_, err := ExpandIncludes(dir, src, nil)
	if err == nil {
		t.Fatal("ожидался цикл, но ошибка отсутствует")
	}
	if !strings.Contains(err.Error(), "цикл") {
		t.Fatalf("ожидалась ошибка о цикле, got: %v", err)
	}
}

func TestExpandIncludesMissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := []byte("<!-- +conflugen-include nope.md -->\n")
	_, err := ExpandIncludes(dir, src, nil)
	if err == nil {
		t.Fatal("ожидалась ошибка, но nil")
	}
	if !strings.Contains(err.Error(), "nope.md") {
		t.Fatalf("ошибка должна упоминать путь, got: %v", err)
	}
}

func TestExpandIncludesNoDirectives(t *testing.T) {
	t.Parallel()

	src := []byte("обычный markdown\nбез include\n")
	out, err := ExpandIncludes(t.TempDir(), src, nil)
	assertNoError(t, err)
	if string(out) != string(src) {
		t.Fatalf("без директив должен быть unchanged; got %q want %q", out, src)
	}
}
