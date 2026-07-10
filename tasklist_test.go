package main

import (
	"strings"
	"testing"
)

func TestTransformTaskListsToConfluence(t *testing.T) {
	t.Parallel()

	t.Run("чистый task-list — конвертируем", func(t *testing.T) {
		t.Parallel()
		in := `<ul>
<li><input checked="" disabled="" type="checkbox" /> done</li>
<li><input disabled="" type="checkbox" /> todo</li>
</ul>`
		out := transformTaskListsToConfluence(in)
		if !strings.Contains(out, "<ac:task-list>") {
			t.Fatalf("ожидался <ac:task-list>:\n%s", out)
		}
		if strings.Count(out, "<ac:task>") != 2 {
			t.Fatalf("ожидалось 2 <ac:task>:\n%s", out)
		}
		if !strings.Contains(out, "<ac:task-status>complete</ac:task-status>") {
			t.Fatalf("нет complete:\n%s", out)
		}
		if !strings.Contains(out, "<ac:task-status>incomplete</ac:task-status>") {
			t.Fatalf("нет incomplete:\n%s", out)
		}
		if !strings.Contains(out, "<ac:task-body>done</ac:task-body>") {
			t.Fatalf("нет body 'done':\n%s", out)
		}
		// task-id уникален в рамках страницы — 1 и 2
		if !strings.Contains(out, "<ac:task-id>1</ac:task-id>") || !strings.Contains(out, "<ac:task-id>2</ac:task-id>") {
			t.Fatalf("ожидались task-id=1 и 2:\n%s", out)
		}
	})

	t.Run("обычный <ul> — не трогаем", func(t *testing.T) {
		t.Parallel()
		in := `<ul>
<li>one</li>
<li>two</li>
</ul>`
		out := transformTaskListsToConfluence(in)
		if out != in {
			t.Fatalf("обычный список не должен меняться:\nin:  %s\nout: %s", in, out)
		}
	})

	t.Run("смешанный список — НЕ конвертируем", func(t *testing.T) {
		t.Parallel()
		// один с input, один без — это не task-list в смысле Confluence
		in := `<ul>
<li><input disabled="" type="checkbox" /> task</li>
<li>regular item</li>
</ul>`
		out := transformTaskListsToConfluence(in)
		if strings.Contains(out, "<ac:task-list>") {
			t.Fatalf("смешанный <ul> не должен превратиться в task-list:\n%s", out)
		}
	})

	t.Run("несколько task-list'ов — task-id continues globally", func(t *testing.T) {
		t.Parallel()
		in := `<ul>
<li><input disabled="" type="checkbox" /> a</li>
</ul>
<p>между</p>
<ul>
<li><input disabled="" type="checkbox" /> b</li>
</ul>`
		out := transformTaskListsToConfluence(in)
		if !strings.Contains(out, "<ac:task-id>1</ac:task-id>") {
			t.Fatalf("нет id=1:\n%s", out)
		}
		if !strings.Contains(out, "<ac:task-id>2</ac:task-id>") {
			t.Fatalf("нет id=2 (счётчик должен продолжаться между списками):\n%s", out)
		}
	})

	t.Run("тело может содержать inline <ac:>", func(t *testing.T) {
		t.Parallel()
		in := `<ul><li><input disabled="" type="checkbox" /> celebrate <ac:emoticon ac:name="smile" /></li></ul>`
		out := transformTaskListsToConfluence(in)
		if !strings.Contains(out, `<ac:task-body>celebrate <ac:emoticon ac:name="smile" /></ac:task-body>`) {
			t.Fatalf("inline <ac:emoticon> должен пройти в task-body:\n%s", out)
		}
	})

	t.Run("UUID детерминированный по телу", func(t *testing.T) {
		t.Parallel()
		in1 := `<ul><li><input disabled="" type="checkbox" /> same body</li></ul>`
		in2 := `<ul><li><input disabled="" type="checkbox" /> same body</li></ul>`
		out1 := transformTaskListsToConfluence(in1)
		out2 := transformTaskListsToConfluence(in2)
		if out1 != out2 {
			t.Fatalf("одинаковый source → разный UUID, идемпотентность сломана:\n%s\n---\n%s", out1, out2)
		}
		// Разное тело → разные UUID
		in3 := `<ul><li><input disabled="" type="checkbox" /> other body</li></ul>`
		out3 := transformTaskListsToConfluence(in3)
		if out1 == out3 {
			t.Fatalf("разный body → должен быть разный UUID")
		}
	})
}

func TestDeterministicUUIDShape(t *testing.T) {
	t.Parallel()
	u := deterministicUUID("hello")
	if len(u) != 36 {
		t.Fatalf("ожидалась длина 36, got %d (%q)", len(u), u)
	}
	parts := strings.Split(u, "-")
	if len(parts) != 5 {
		t.Fatalf("ожидалось 5 групп через -, got %d (%q)", len(parts), u)
	}
	wantLens := []int{8, 4, 4, 4, 12}
	for i, p := range parts {
		if len(p) != wantLens[i] {
			t.Fatalf("группа %d: ожидалась длина %d, got %d (%q)", i, wantLens[i], len(p), p)
		}
	}
}
