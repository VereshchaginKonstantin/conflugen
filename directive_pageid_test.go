package main

import "testing"

// page-id парсится и снимает требование parent-id/parent.
func TestDirectivePageID(t *testing.T) {
	t.Parallel()

	t.Run("page-id без parent — валидно", func(t *testing.T) {
		t.Parallel()

		d, _, err := ParseDirective([]byte(
			`<!-- +conflugen page-id=1007007496 space-key=riskdev title="X" -->` + "\nbody",
		))
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if d.PageID != "1007007496" {
			t.Fatalf("PageID = %q, ждал 1007007496", d.PageID)
		}
	})

	t.Run("без page-id/parent-id/parent — ошибка", func(t *testing.T) {
		t.Parallel()

		_, _, err := ParseDirective([]byte(
			`<!-- +conflugen space-key=riskdev title="X" -->` + "\nbody",
		))
		if err == nil {
			t.Fatal("ждал ошибку об отсутствии page-id/parent-id/parent")
		}
	})
}
