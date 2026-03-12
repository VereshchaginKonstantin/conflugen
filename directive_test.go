package main

import (
	"testing"
)

func TestParseDirective(t *testing.T) {
	t.Parallel()

	t.Run("однострочная директива", func(t *testing.T) {
		t.Parallel()

		content := []byte(`<!-- +conflugen parent-id=123456 space-key=OB title="Мой заголовок" -->
# Содержимое
текст`)

		d, cleaned, err := ParseDirective(content)

		assertNoError(t, err)
		assertNotNil(t, d)
		assertEqual(t, "123456", d.ParentID)
		assertEqual(t, "OB", d.SpaceKey)
		assertEqual(t, "Мой заголовок", d.Title)
		assertContains(t, string(cleaned), "# Содержимое")
		assertNotContains(t, string(cleaned), "+conflugen")
	})

	t.Run("многострочные директивы", func(t *testing.T) {
		t.Parallel()

		content := []byte(`<!-- +conflugen parent-id=123456 space-key=OB -->
<!-- +conflugen title="Кастомный заголовок" -->
# Документ
`)

		d, cleaned, err := ParseDirective(content)

		assertNoError(t, err)
		assertNotNil(t, d)
		assertEqual(t, "123456", d.ParentID)
		assertEqual(t, "OB", d.SpaceKey)
		assertEqual(t, "Кастомный заголовок", d.Title)
		assertContains(t, string(cleaned), "# Документ")
	})

	t.Run("без title — остаётся пустым", func(t *testing.T) {
		t.Parallel()

		content := []byte(`<!-- +conflugen parent-id=789 space-key=DEV -->
# Doc`)

		d, _, err := ParseDirective(content)

		assertNoError(t, err)
		assertNotNil(t, d)
		assertEqual(t, "789", d.ParentID)
		assertEqual(t, "DEV", d.SpaceKey)
		assertEqual(t, "", d.Title)
	})

	t.Run("нет директивы — возвращает nil", func(t *testing.T) {
		t.Parallel()

		content := []byte(`# Обычный документ
без директив`)

		d, cleaned, err := ParseDirective(content)

		assertNoError(t, err)
		if d != nil {
			t.Fatal("expected nil directive")
		}
		assertEqual(t, string(content), string(cleaned))
	})

	t.Run("отсутствует parent-id", func(t *testing.T) {
		t.Parallel()

		content := []byte(`<!-- +conflugen space-key=OB -->
# Doc`)

		_, _, err := ParseDirective(content)

		assertError(t, err)
		assertContains(t, err.Error(), "parent-id")
	})

	t.Run("отсутствует space-key", func(t *testing.T) {
		t.Parallel()

		content := []byte(`<!-- +conflugen parent-id=123 -->
# Doc`)

		_, _, err := ParseDirective(content)

		assertError(t, err)
		assertContains(t, err.Error(), "space-key")
	})

	t.Run("неизвестный параметр", func(t *testing.T) {
		t.Parallel()

		content := []byte(`<!-- +conflugen parent-id=123 space-key=OB unknown=val -->`)

		_, _, err := ParseDirective(content)

		assertError(t, err)
		assertContains(t, err.Error(), "unknown")
	})

	t.Run("незакрытая кавычка", func(t *testing.T) {
		t.Parallel()

		content := []byte(`<!-- +conflugen parent-id=123 space-key=OB title="незакрытая -->`)

		_, _, err := ParseDirective(content)

		assertError(t, err)
		assertContains(t, err.Error(), "unclosed quote")
	})

	t.Run("title с пробелами в кавычках", func(t *testing.T) {
		t.Parallel()

		content := []byte(`<!-- +conflugen parent-id=1 space-key=X title="Название с пробелами и (скобками)" -->
text`)

		d, _, err := ParseDirective(content)

		assertNoError(t, err)
		assertEqual(t, "Название с пробелами и (скобками)", d.Title)
	})

	t.Run("пустой контент", func(t *testing.T) {
		t.Parallel()

		d, cleaned, err := ParseDirective([]byte{})

		assertNoError(t, err)
		if d != nil {
			t.Fatal("expected nil directive")
		}
		assertEqual(t, 0, len(cleaned))
	})
}

// Минимальные assert-хелперы без зависимости от testify
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func assertNotNil(t *testing.T, v interface{}) {
	t.Helper()
	if v == nil {
		t.Fatal("expected non-nil value")
	}
}

func assertEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	if expected != actual {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if len(s) == 0 || len(substr) == 0 {
		if len(substr) > 0 {
			t.Fatalf("expected %q to contain %q", s, substr)
		}
		return
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return
		}
	}
	t.Fatalf("expected %q to contain %q", s, substr)
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			t.Fatalf("expected %q to NOT contain %q", s, substr)
		}
	}
}
