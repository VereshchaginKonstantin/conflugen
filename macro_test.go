package main

import (
	"strings"
	"testing"
)

func TestExtractMacros(t *testing.T) {
	t.Parallel()

	t.Run("один макрос — изымается, остаётся в списке", func(t *testing.T) {
		t.Parallel()
		src := []byte("# Doc\n\n<!-- +conflugen-macro JIRA-\\d+ => <a href=\"$0\">$0</a> -->\n\nJIRA-123 reported\n")
		macros, out, err := ExtractMacros(src)
		assertNoError(t, err)
		assertEqual(t, 1, len(macros))
		if !strings.Contains(macros[0].Pattern.String(), "JIRA-") {
			t.Fatalf("pattern не сохранился: %q", macros[0].Pattern.String())
		}
		if !strings.Contains(macros[0].Template, "<a href=") {
			t.Fatalf("template потерян: %q", macros[0].Template)
		}
		if strings.Contains(string(out), "+conflugen-macro") {
			t.Fatalf("директива не удалена:\n%s", out)
		}
	})

	t.Run("несколько макросов — порядок сохраняется", func(t *testing.T) {
		t.Parallel()
		src := []byte("<!-- +conflugen-macro foo => A -->\n<!-- +conflugen-macro bar => B -->\n")
		macros, _, err := ExtractMacros(src)
		assertNoError(t, err)
		assertEqual(t, 2, len(macros))
		assertEqual(t, "foo", macros[0].Pattern.String())
		assertEqual(t, "bar", macros[1].Pattern.String())
	})

	t.Run("битый regex — ошибка", func(t *testing.T) {
		t.Parallel()
		_, _, err := ExtractMacros([]byte(`<!-- +conflugen-macro [unclosed => X -->`))
		assertError(t, err)
	})

	t.Run("нет директив — пусто", func(t *testing.T) {
		t.Parallel()
		macros, out, err := ExtractMacros([]byte("обычный markdown"))
		assertNoError(t, err)
		assertEqual(t, 0, len(macros))
		assertEqual(t, "обычный markdown", string(out))
	})
}

func TestApplyMacrosCaptures(t *testing.T) {
	t.Parallel()

	macros, _, err := ExtractMacros([]byte(`<!-- +conflugen-macro (\d+)-(\d+) => [$1+$2] -->`))
	assertNoError(t, err)
	out := ApplyMacros([]byte("числа 12-34 и 5-6"), macros)
	if string(out) != "числа [12+34] и [5+6]" {
		t.Fatalf("ожидалось 'числа [12+34] и [5+6]', got %q", string(out))
	}
}

func TestApplyMacrosOrder(t *testing.T) {
	t.Parallel()

	// два макроса; второй опирается на результат первого
	src := []byte("<!-- +conflugen-macro X => Y -->\n<!-- +conflugen-macro Y => Z -->\nX в тексте")
	macros, body, err := ExtractMacros(src)
	assertNoError(t, err)
	out := ApplyMacros(body, macros)
	if !strings.Contains(string(out), "Z в тексте") {
		t.Fatalf("ожидалось 'Z в тексте' (X→Y→Z), got %q", string(out))
	}
}

func TestEnableStdlibTOC(t *testing.T) {
	t.Parallel()

	src := []byte("<!-- +conflugen-use toc -->\n# Doc\n\n[[toc]]\n")
	macros, body, err := EnableStdlibPacks(src, nil)
	assertNoError(t, err)
	out := ApplyMacros(body, macros)
	if !strings.Contains(string(out), `ac:name="toc"`) {
		t.Fatalf("ожидался toc макрос в выводе:\n%s", string(out))
	}
	if strings.Contains(string(out), "[[toc]]") || strings.Contains(string(out), "+conflugen-use") {
		t.Fatalf("незаменённые маркеры в выводе:\n%s", string(out))
	}
}

func TestEnableStdlibJiraDefault(t *testing.T) {
	t.Parallel()

	src := []byte("<!-- +conflugen-use jira -->\nfix JIRA-42 ASAP")
	macros, body, err := EnableStdlibPacks(src, nil)
	assertNoError(t, err)
	out := ApplyMacros(body, macros)
	if !strings.Contains(string(out), `ac:name="jira"`) {
		t.Fatalf("нет jira макроса:\n%s", string(out))
	}
	if !strings.Contains(string(out), `<ac:parameter ac:name="key">JIRA-42</ac:parameter>`) {
		t.Fatalf("нет capture в шаблоне:\n%s", string(out))
	}
}

func TestEnableStdlibJiraCustomProject(t *testing.T) {
	t.Parallel()

	src := []byte("<!-- +conflugen-use jira project=PLAT -->\nfix PLAT-7 — JIRA-1 не трогать")
	macros, body, err := EnableStdlibPacks(src, nil)
	assertNoError(t, err)
	out := ApplyMacros(body, macros)
	if !strings.Contains(string(out), `<ac:parameter ac:name="key">PLAT-7</ac:parameter>`) {
		t.Fatalf("PLAT-7 не залинкован:\n%s", string(out))
	}
	if strings.Contains(string(out), `JIRA-1</ac:parameter>`) {
		t.Fatalf("JIRA-1 не должен трогаться при project=PLAT:\n%s", string(out))
	}
}

func TestEnableStdlibStatus(t *testing.T) {
	t.Parallel()

	src := []byte("<!-- +conflugen-use status -->\n[status:Green Done]")
	macros, body, err := EnableStdlibPacks(src, nil)
	assertNoError(t, err)
	out := ApplyMacros(body, macros)
	for _, want := range []string{
		`ac:name="status"`,
		`<ac:parameter ac:name="colour">Green</ac:parameter>`,
		`<ac:parameter ac:name="title">Done</ac:parameter>`,
	} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("нет %q в:\n%s", want, string(out))
		}
	}
}

func TestEnableStdlibBox(t *testing.T) {
	t.Parallel()

	src := []byte("<!-- +conflugen-use box -->\n[info: важное сообщение]\n[warning: осторожно!]")
	macros, body, err := EnableStdlibPacks(src, nil)
	assertNoError(t, err)
	out := ApplyMacros(body, macros)
	for _, want := range []string{
		`ac:name="info"`, `<p>важное сообщение</p>`,
		`ac:name="warning"`, `<p>осторожно!</p>`,
	} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("нет %q в:\n%s", want, string(out))
		}
	}
}

func TestEnableStdlibUnknownPack(t *testing.T) {
	t.Parallel()
	_, _, err := EnableStdlibPacks([]byte(`<!-- +conflugen-use какой-то-нерсуществующий -->`), nil)
	assertError(t, err)
	if !strings.Contains(err.Error(), "неизвестный") {
		t.Fatalf("ожидалась ошибка про неизвестный пакет, got: %v", err)
	}
}
