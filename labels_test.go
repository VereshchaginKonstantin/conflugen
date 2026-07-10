package main

import (
	"sort"
	"testing"
)

func TestSyncLabels(t *testing.T) {
	t.Parallel()

	t.Run("добавляет недостающие", func(t *testing.T) {
		t.Parallel()
		api := newStubAPI()
		api.labels["p"] = []string{"a"}

		err := syncLabels(api, "p", []string{"a", "b"})
		assertNoError(t, err)
		sort.Strings(api.addedLabels["p"])
		if got := api.addedLabels["p"]; len(got) != 1 || got[0] != "b" {
			t.Fatalf("ожидалось добавление b, got %v", got)
		}
		if len(api.delLabels["p"]) != 0 {
			t.Fatalf("ничего удалять не должно, удалили %v", api.delLabels["p"])
		}
	})

	t.Run("удаляет лишние", func(t *testing.T) {
		t.Parallel()
		api := newStubAPI()
		api.labels["p"] = []string{"a", "b"}

		err := syncLabels(api, "p", []string{"a"})
		assertNoError(t, err)
		if got := api.delLabels["p"]; len(got) != 1 || got[0] != "b" {
			t.Fatalf("ожидалось удаление b, got %v", got)
		}
		if len(api.addedLabels["p"]) != 0 {
			t.Fatalf("ничего добавлять не должно, добавили %v", api.addedLabels["p"])
		}
	})

	t.Run("идемпотентно — desired == current → ничего", func(t *testing.T) {
		t.Parallel()
		api := newStubAPI()
		api.labels["p"] = []string{"a", "b"}

		err := syncLabels(api, "p", []string{"b", "a"})
		assertNoError(t, err)
		if len(api.addedLabels["p"]) != 0 || len(api.delLabels["p"]) != 0 {
			t.Fatalf("ожидался no-op, added=%v del=%v", api.addedLabels["p"], api.delLabels["p"])
		}
	})

	t.Run("пустой desired — удаляет всё текущее", func(t *testing.T) {
		t.Parallel()
		api := newStubAPI()
		api.labels["p"] = []string{"a", "b"}

		err := syncLabels(api, "p", nil)
		assertNoError(t, err)
		sort.Strings(api.delLabels["p"])
		if got := api.delLabels["p"]; len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Fatalf("ожидалось удаление [a b], got %v", got)
		}
	})

	t.Run("пустой current — добавляет всё desired", func(t *testing.T) {
		t.Parallel()
		api := newStubAPI()

		err := syncLabels(api, "p", []string{"x", "y"})
		assertNoError(t, err)
		sort.Strings(api.addedLabels["p"])
		if got := api.addedLabels["p"]; len(got) != 2 || got[0] != "x" || got[1] != "y" {
			t.Fatalf("ожидалось добавление [x y], got %v", got)
		}
	})
}

func TestPublishPageUsesType(t *testing.T) {
	t.Parallel()

	t.Run("blogpost", func(t *testing.T) {
		t.Parallel()
		api := newStubAPI()
		_, err := publishPage(api, nil, "", "", "p", "OB", "Hello", "<p>x</p>", "h", "blogpost", false)
		assertNoError(t, err)
		assertEqual(t, 1, len(api.created))
		assertEqual(t, "blogpost", api.created[0].Type)
	})

	t.Run("дефолт — page", func(t *testing.T) {
		t.Parallel()
		api := newStubAPI()
		_, err := publishPage(api, nil, "", "", "p", "OB", "Hello", "<p>x</p>", "h", "", false)
		assertNoError(t, err)
		assertEqual(t, 1, len(api.created))
		assertEqual(t, "page", api.created[0].Type)
	})
}
