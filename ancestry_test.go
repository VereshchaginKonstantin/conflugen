package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeAncestry — мок ancestryAPI с in-memory иерархией и записью созданий.
type fakeAncestry struct {
	top        map[string]string            // title -> id (верхний уровень)
	children   map[string]map[string]string // parentID -> title -> id
	created    []string                     // "parentID/title" в порядке создания
	seq        int
	failFind   bool
	failCreate bool
}

func newFakeAncestry() *fakeAncestry {
	return &fakeAncestry{top: map[string]string{}, children: map[string]map[string]string{}}
}

func (f *fakeAncestry) FindPageByTitle(_ /*space*/, title string) (string, bool, error) {
	if f.failFind {
		return "", false, errors.New("find boom")
	}
	id, ok := f.top[title]
	return id, ok, nil
}

func (f *fakeAncestry) FindChildByTitle(parentID, title string) (string, bool, error) {
	if f.failFind {
		return "", false, errors.New("find boom")
	}
	if m := f.children[parentID]; m != nil {
		id, ok := m[title]
		return id, ok, nil
	}
	return "", false, nil
}

func (f *fakeAncestry) CreateEmptyPage(_ /*space*/, parentID, title string) (string, error) {
	if f.failCreate {
		return "", errors.New("create boom")
	}
	f.seq++
	id := fmt.Sprintf("new-%d", f.seq)
	f.created = append(f.created, parentID+"/"+title)
	if parentID == "" {
		f.top[title] = id
	} else {
		if f.children[parentID] == nil {
			f.children[parentID] = map[string]string{}
		}
		f.children[parentID][title] = id
	}
	return id, nil
}

func TestResolveParentID(t *testing.T) {
	t.Parallel()

	t.Run("вся цепочка существует — без создания", func(t *testing.T) {
		t.Parallel()
		f := newFakeAncestry()
		f.top["A"] = "1"
		f.children["1"] = map[string]string{"B": "2"}

		id, err := ResolveParentID(f, "SP", []string{"A", "B"})

		require.NoError(t, err)
		require.Equal(t, "2", id)
		require.Empty(t, f.created)
	})

	t.Run("отсутствует нижний — создаётся под существующим", func(t *testing.T) {
		t.Parallel()
		f := newFakeAncestry()
		f.top["A"] = "1"

		id, err := ResolveParentID(f, "SP", []string{"A", "B"})

		require.NoError(t, err)
		require.Equal(t, "new-1", id)
		require.Equal(t, []string{"1/B"}, f.created)
	})

	t.Run("отсутствует вся цепочка — создаётся от корня", func(t *testing.T) {
		t.Parallel()
		f := newFakeAncestry()

		id, err := ResolveParentID(f, "SP", []string{"A", "B"})

		require.NoError(t, err)
		require.Equal(t, "new-2", id)
		require.Equal(t, []string{"/A", "new-1/B"}, f.created)
	})

	t.Run("один родитель, существует", func(t *testing.T) {
		t.Parallel()
		f := newFakeAncestry()
		f.top["A"] = "42"

		id, err := ResolveParentID(f, "SP", []string{"A"})

		require.NoError(t, err)
		require.Equal(t, "42", id)
		require.Empty(t, f.created)
	})

	t.Run("пустой список — ошибка", func(t *testing.T) {
		t.Parallel()
		_, err := ResolveParentID(newFakeAncestry(), "SP", nil)
		require.Error(t, err)
	})

	t.Run("ошибка поиска пробрасывается", func(t *testing.T) {
		t.Parallel()
		f := newFakeAncestry()
		f.failFind = true
		_, err := ResolveParentID(f, "SP", []string{"A"})
		require.Error(t, err)
	})

	t.Run("ошибка создания пробрасывается", func(t *testing.T) {
		t.Parallel()
		f := newFakeAncestry()
		f.failCreate = true
		_, err := ResolveParentID(f, "SP", []string{"A"})
		require.Error(t, err)
	})
}
