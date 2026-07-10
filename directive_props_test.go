package main

import "testing"

func TestParseDirectiveLabels(t *testing.T) {
	t.Parallel()

	t.Run("одна метка", func(t *testing.T) {
		t.Parallel()
		d, _, err := ParseDirective([]byte(`<!-- +conflugen parent-id=1 space-key=OB label=arch -->
# Doc`))
		assertNoError(t, err)
		assertEqual(t, 1, len(d.Labels))
		assertEqual(t, "arch", d.Labels[0])
	})

	t.Run("несколько меток через повторение", func(t *testing.T) {
		t.Parallel()
		d, _, err := ParseDirective([]byte(`<!-- +conflugen parent-id=1 space-key=OB label=arch label=docs -->
<!-- +conflugen label=team -->
# Doc`))
		assertNoError(t, err)
		assertEqual(t, 3, len(d.Labels))
		assertEqual(t, "arch", d.Labels[0])
		assertEqual(t, "docs", d.Labels[1])
		assertEqual(t, "team", d.Labels[2])
	})

	t.Run("метка с пробелами в кавычках", func(t *testing.T) {
		t.Parallel()
		d, _, err := ParseDirective([]byte(`<!-- +conflugen parent-id=1 space-key=OB label="team-platform" -->
# Doc`))
		assertNoError(t, err)
		assertEqual(t, 1, len(d.Labels))
		assertEqual(t, "team-platform", d.Labels[0])
	})

	t.Run("без меток — пусто", func(t *testing.T) {
		t.Parallel()
		d, _, err := ParseDirective([]byte(`<!-- +conflugen parent-id=1 space-key=OB -->
# Doc`))
		assertNoError(t, err)
		assertEqual(t, 0, len(d.Labels))
	})
}

func TestParseDirectiveType(t *testing.T) {
	t.Parallel()

	t.Run("type=page — ок", func(t *testing.T) {
		t.Parallel()
		d, _, err := ParseDirective([]byte(`<!-- +conflugen parent-id=1 space-key=OB type=page -->
# Doc`))
		assertNoError(t, err)
		assertEqual(t, "page", d.Type)
	})

	t.Run("type=blogpost — ок", func(t *testing.T) {
		t.Parallel()
		d, _, err := ParseDirective([]byte(`<!-- +conflugen parent-id=1 space-key=OB type=blogpost -->
# Doc`))
		assertNoError(t, err)
		assertEqual(t, "blogpost", d.Type)
	})

	t.Run("type не задан — пусто (createPage применит дефолт)", func(t *testing.T) {
		t.Parallel()
		d, _, err := ParseDirective([]byte(`<!-- +conflugen parent-id=1 space-key=OB -->
# Doc`))
		assertNoError(t, err)
		assertEqual(t, "", d.Type)
	})

	t.Run("неизвестный type — ошибка", func(t *testing.T) {
		t.Parallel()
		_, _, err := ParseDirective([]byte(`<!-- +conflugen parent-id=1 space-key=OB type=widget -->
# Doc`))
		assertError(t, err)
		assertContains(t, err.Error(), "type")
	})
}

func TestParseDirectiveContentAppearance(t *testing.T) {
	t.Parallel()

	t.Run("full-width", func(t *testing.T) {
		t.Parallel()
		d, _, err := ParseDirective([]byte(`<!-- +conflugen parent-id=1 space-key=OB content-appearance=full-width -->
# Doc`))
		assertNoError(t, err)
		assertEqual(t, "full-width", d.ContentAppearance)
	})

	t.Run("fixed-width", func(t *testing.T) {
		t.Parallel()
		d, _, err := ParseDirective([]byte(`<!-- +conflugen parent-id=1 space-key=OB content-appearance=fixed-width -->
# Doc`))
		assertNoError(t, err)
		assertEqual(t, "fixed-width", d.ContentAppearance)
	})

	t.Run("неизвестное значение — ошибка", func(t *testing.T) {
		t.Parallel()
		_, _, err := ParseDirective([]byte(`<!-- +conflugen parent-id=1 space-key=OB content-appearance=narrow -->
# Doc`))
		assertError(t, err)
		assertContains(t, err.Error(), "content-appearance")
	})
}
