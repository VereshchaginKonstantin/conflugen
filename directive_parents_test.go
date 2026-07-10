package main

import "testing"

func TestParseDirectiveParents(t *testing.T) {
	t.Parallel()

	t.Run("один parent по имени", func(t *testing.T) {
		t.Parallel()
		d, _, err := ParseDirective([]byte(`<!-- +conflugen space-key=OB parent="Команда" title="Док" -->
# Doc`))
		assertNoError(t, err)
		assertNotNil(t, d)
		assertEqual(t, "", d.ParentID)
		assertEqual(t, 1, len(d.Parents))
		assertEqual(t, "Команда", d.Parents[0])
	})

	t.Run("несколько parent — иерархия", func(t *testing.T) {
		t.Parallel()
		d, _, err := ParseDirective([]byte(`<!-- +conflugen space-key=OB parent="A" -->
<!-- +conflugen parent="B" -->
# Doc`))
		assertNoError(t, err)
		assertEqual(t, 2, len(d.Parents))
		assertEqual(t, "A", d.Parents[0])
		assertEqual(t, "B", d.Parents[1])
	})

	t.Run("parent-id и parent одновременно — ошибка", func(t *testing.T) {
		t.Parallel()
		_, _, err := ParseDirective([]byte(`<!-- +conflugen space-key=OB parent-id=123 parent="A" -->
# Doc`))
		assertError(t, err)
	})

	t.Run("ни parent-id, ни parent — ошибка", func(t *testing.T) {
		t.Parallel()
		_, _, err := ParseDirective([]byte(`<!-- +conflugen space-key=OB title="Док" -->
# Doc`))
		assertError(t, err)
	})

	t.Run("back-compat: только parent-id — ок", func(t *testing.T) {
		t.Parallel()
		d, _, err := ParseDirective([]byte(`<!-- +conflugen parent-id=123 space-key=OB -->
# Doc`))
		assertNoError(t, err)
		assertEqual(t, "123", d.ParentID)
		assertEqual(t, 0, len(d.Parents))
	})
}
