package extensions

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

// ──────────────────────────── Хелперы ────────────────────────────

// renderLayout прогоняет src через goldmark с одним LayoutExtension.
func renderLayout(t *testing.T, src string) (string, error) {
	t.Helper()
	col := NewLayoutCollector()
	md := goldmark.New(
		goldmark.WithExtensions(&LayoutExtension{Collector: col}),
		goldmark.WithRendererOptions(html.WithXHTML(), html.WithHardWraps()),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return buf.String(), col.Err()
}

// norm убирает переводы строк, чтобы сравнивать структуру без учёта вёрстки.
func norm(s string) string {
	return strings.ReplaceAll(s, "\n", "")
}

type errString string

func (e errString) Error() string { return string(e) }

// ──────────────────────────── Чистые хелперы ────────────────────────────

func TestParseFenceLine(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		fence   bool
		isClose bool
		kind    string
		typ     string
		wantErr bool
	}{
		{"not a fence", "обычный текст", false, false, "", "", false},
		{"bare close", ":::", true, true, "", "", false},
		{"bare close trailing spaces", ":::   ", true, true, "", "", false},
		{"columns with type", "::: columns type=two_equal", true, false, "columns", "two_equal", false},
		{"columns no type", "::: columns", true, false, "columns", "", false},
		{"cell", "::: cell", true, false, "cell", "", false},
		{"four colons not ours", ":::: columns", false, false, "", "", false},
		{"bad token", "::: columns wat", true, false, "columns", "", true},
		{"unknown attr", "::: columns size=2", true, false, "columns", "", true},
		{"leading spaces", "  ::: cell", true, false, "cell", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseFenceLine([]byte(c.in))
			if got.isFence != c.fence || got.isClose != c.isClose ||
				got.kind != c.kind || got.typ != c.typ || (got.err != nil) != c.wantErr {
				t.Fatalf("parseFenceLine(%q) = %+v", c.in, got)
			}
		})
	}
}

func TestInferLayoutType(t *testing.T) {
	cases := []struct {
		cells   int
		want    string
		wantErr bool
	}{
		{1, "single", false},
		{2, "two_equal", false},
		{3, "three_equal", false},
		{0, "", true},
		{4, "", true},
	}
	for _, c := range cases {
		got, err := inferLayoutType(c.cells)
		if got != c.want || (err != nil) != c.wantErr {
			t.Fatalf("inferLayoutType(%d) = %q, err=%v", c.cells, got, err)
		}
	}
}

func TestLayoutCollector(t *testing.T) {
	col := NewLayoutCollector()
	if col.Err() != nil {
		t.Fatal("пустой коллектор должен возвращать nil")
	}
	col.Add(nil)
	if col.Err() != nil {
		t.Fatal("Add(nil) не должен добавлять ошибку")
	}
	col.Add(errString("первая"))
	col.Add(errString("вторая"))
	err := col.Err()
	if err == nil || !strings.Contains(err.Error(), "первая") || !strings.Contains(err.Error(), "вторая") {
		t.Fatalf("Err() = %v", err)
	}
	col.Reset()
	if col.Err() != nil {
		t.Fatal("после Reset() ошибок быть не должно")
	}
}

// ──────────────────────────── Happy-path ────────────────────────────

func TestLayoutTwoEqual(t *testing.T) {
	src := "::: columns type=two_equal\n" +
		"::: cell\n" +
		"A\n" +
		":::\n" +
		"::: cell\n" +
		"B\n" +
		":::\n" +
		":::\n"
	out, err := renderLayout(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	want := `<ac:layout><ac:layout-section ac:type="two_equal">` +
		`<ac:layout-cell><p>A</p></ac:layout-cell>` +
		`<ac:layout-cell><p>B</p></ac:layout-cell>` +
		`</ac:layout-section></ac:layout>`
	if norm(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", norm(out), want)
	}
}

func TestLayoutThreeEqualExplicit(t *testing.T) {
	src := "::: columns type=three_equal\n" +
		"::: cell\nA\n:::\n::: cell\nB\n:::\n::: cell\nC\n:::\n:::\n"
	out, err := renderLayout(t, src)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	want := `<ac:layout><ac:layout-section ac:type="three_equal">` +
		`<ac:layout-cell><p>A</p></ac:layout-cell>` +
		`<ac:layout-cell><p>B</p></ac:layout-cell>` +
		`<ac:layout-cell><p>C</p></ac:layout-cell>` +
		`</ac:layout-section></ac:layout>`
	if norm(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", norm(out), want)
	}
}

func TestLayoutInferType(t *testing.T) {
	// type= опущен → выводится из числа ячеек.
	cases := []struct {
		cells int
		typ   string
	}{
		{1, "single"},
		{2, "two_equal"},
		{3, "three_equal"},
	}
	for _, c := range cases {
		var b strings.Builder
		b.WriteString("::: columns\n")
		for i := 0; i < c.cells; i++ {
			b.WriteString("::: cell\nX\n:::\n")
		}
		b.WriteString(":::\n")
		out, err := renderLayout(t, b.String())
		if err != nil {
			t.Fatalf("%d ячеек: ошибка %v", c.cells, err)
		}
		if !strings.Contains(norm(out), `<ac:layout-section ac:type="`+c.typ+`">`) {
			t.Fatalf("%d ячеек: ожидал type=%s, got:\n%s", c.cells, c.typ, norm(out))
		}
	}
}

func TestLayoutMarkdownInsideCell(t *testing.T) {
	// Внутри ячейки — обычный markdown; он парсится goldmark штатно.
	src := "::: columns type=two_equal\n" +
		"::: cell\n## Заголовок\n\n- раз\n- два\n:::\n" +
		"::: cell\n`code`\n:::\n:::\n"
	out, err := renderLayout(t, src)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	n := norm(out)
	for _, frag := range []string{
		`<ac:layout-cell><h2`,
		`<ul>`, `<li>раз`, `<li>два`,
		`<ac:layout-cell><p><code>code</code></p></ac:layout-cell>`,
	} {
		if !strings.Contains(n, frag) {
			t.Fatalf("нет фрагмента %q в:\n%s", frag, n)
		}
	}
}

// ──────────────────────────── Обёртка тела ────────────────────────────

func TestLayoutWrapsLooseContent(t *testing.T) {
	src := "Вступление\n\n" +
		"::: columns type=two_equal\n::: cell\nA\n:::\n::: cell\nB\n:::\n:::\n\n" +
		"Заключение\n"
	out, err := renderLayout(t, src)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	want := `<ac:layout>` +
		`<ac:layout-section ac:type="single"><ac:layout-cell><p>Вступление</p></ac:layout-cell></ac:layout-section>` +
		`<ac:layout-section ac:type="two_equal">` +
		`<ac:layout-cell><p>A</p></ac:layout-cell>` +
		`<ac:layout-cell><p>B</p></ac:layout-cell>` +
		`</ac:layout-section>` +
		`<ac:layout-section ac:type="single"><ac:layout-cell><p>Заключение</p></ac:layout-cell></ac:layout-section>` +
		`</ac:layout>`
	if norm(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", norm(out), want)
	}
}

func TestLayoutAbsentLeavesDocFlat(t *testing.T) {
	// Без layout-блоков документ рендерится плоско — нулевая регрессия.
	out, err := renderLayout(t, "Просто абзац\n")
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if strings.Contains(out, "ac:layout") {
		t.Fatalf("не должно быть обёртки:\n%s", out)
	}
	if !strings.Contains(norm(out), "<p>Просто абзац</p>") {
		t.Fatalf("ожидал плоский абзац, got:\n%s", out)
	}
}

// ──────────────────────────── Ветки ошибок ────────────────────────────

func TestLayoutErrors(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		errMatch string
	}{
		{
			"cell вне columns",
			"::: cell\nA\n:::\n",
			"'::: cell' вне",
		},
		{
			"вложенный columns",
			"::: columns type=single\n::: cell\n::: columns type=single\n::: cell\nA\n:::\n:::\n:::\n:::\n",
			"вложенный",
		},
		{
			"больше 3 ячеек без типа",
			"::: columns\n::: cell\nA\n:::\n::: cell\nB\n:::\n::: cell\nC\n:::\n::: cell\nD\n:::\n:::\n",
			"допустимо 1–3",
		},
		{
			"неизвестный тип",
			"::: columns type=four_equal\n::: cell\nA\n:::\n:::\n",
			"неизвестный type",
		},
		{
			"число ячеек не соответствует типу",
			"::: columns type=single\n::: cell\nA\n:::\n::: cell\nB\n:::\n:::\n",
			"ожидает 1 ячеек, найдено 2",
		},
		{
			"незакрытая секция",
			"::: columns type=single\n::: cell\nA\n:::\n",
			"незакрытый '::: columns'",
		},
		{
			"лишний закрывающий фенс",
			"текст\n:::\n",
			"без открытого блока",
		},
		{
			"мусор в info-string",
			"::: columns wat\n::: cell\nA\n:::\n:::\n",
			"непонятный токен",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := renderLayout(t, c.src)
			if err == nil {
				t.Fatalf("ожидал ошибку с %q, got nil", c.errMatch)
			}
			if !strings.Contains(err.Error(), c.errMatch) {
				t.Fatalf("ошибка %q не содержит %q", err.Error(), c.errMatch)
			}
		})
	}
}
