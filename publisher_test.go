package main

import (
	"fmt"
	"testing"

	goconfluence "github.com/virtomize/confluence-go-api"
)

type stubAPI struct {
	pages    map[string]*goconfluence.Content
	children map[string]*goconfluence.Search
	created  []*goconfluence.Content
	updated  []*goconfluence.Content

	createErr error
	getErr    error
	childErr  error
	updateErr error
}

func newStubAPI() *stubAPI {
	return &stubAPI{
		pages:    make(map[string]*goconfluence.Content),
		children: make(map[string]*goconfluence.Search),
	}
}

func (s *stubAPI) GetContentByID(id string, _ goconfluence.ContentQuery) (*goconfluence.Content, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if page, ok := s.pages[id]; ok {
		return page, nil
	}
	return &goconfluence.Content{
		ID:      id,
		Title:   "Page " + id,
		Body:    goconfluence.Body{Storage: goconfluence.Storage{Value: ""}},
		Version: &goconfluence.Version{Number: 1},
	}, nil
}

func (s *stubAPI) GetChildPages(id string) (*goconfluence.Search, error) {
	if s.childErr != nil {
		return nil, s.childErr
	}
	if children, ok := s.children[id]; ok {
		return children, nil
	}
	return &goconfluence.Search{Results: []goconfluence.Results{}}, nil
}

func (s *stubAPI) CreateContent(content *goconfluence.Content) (*goconfluence.Content, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	result := &goconfluence.Content{
		ID:    "new-" + content.Title,
		Title: content.Title,
	}
	s.created = append(s.created, result)
	return result, nil
}

func (s *stubAPI) UpdateContent(content *goconfluence.Content) (*goconfluence.Content, error) {
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	s.updated = append(s.updated, content)
	return content, nil
}

func TestExtractStoredHash(t *testing.T) {
	t.Parallel()

	t.Run("хеш найден", func(t *testing.T) {
		t.Parallel()

		hash := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
		html := `<p>content</p>conflugen-hash:` + hash

		result := extractStoredHash(html)
		assertEqual(t, hash, result)
	})

	t.Run("хеш не найден", func(t *testing.T) {
		t.Parallel()

		result := extractStoredHash(`<p>no hash</p>`)
		assertEqual(t, "", result)
	})
}

func TestPublishPage(t *testing.T) {
	t.Parallel()

	t.Run("создание новой страницы", func(t *testing.T) {
		t.Parallel()

		api := newStubAPI()

		err := publishPage(api, "parent-1", "OB", "New Page", "<p>content</p>", "hash123", false)

		assertNoError(t, err)
		assertEqual(t, 1, len(api.created))
		assertEqual(t, "New Page", api.created[0].Title)
	})

	t.Run("обновление существующей страницы", func(t *testing.T) {
		t.Parallel()

		api := newStubAPI()
		api.children["parent-1"] = &goconfluence.Search{
			Results: []goconfluence.Results{
				{ID: "page-1", Title: "Existing"},
			},
		}
		api.pages["page-1"] = &goconfluence.Content{
			ID:      "page-1",
			Title:   "Existing",
			Body:    goconfluence.Body{Storage: goconfluence.Storage{Value: "old"}},
			Version: &goconfluence.Version{Number: 1},
		}

		err := publishPage(api, "parent-1", "OB", "Existing", "<p>new</p>", "newhash", false)

		assertNoError(t, err)
		assertEqual(t, 0, len(api.created))
		assertEqual(t, 1, len(api.updated))
	})

	t.Run("пропуск при одинаковом хеше", func(t *testing.T) {
		t.Parallel()

		hash := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
		api := newStubAPI()
		api.children["parent-1"] = &goconfluence.Search{
			Results: []goconfluence.Results{
				{ID: "page-1", Title: "Page"},
			},
		}
		api.pages["page-1"] = &goconfluence.Content{
			ID:      "page-1",
			Title:   "Page",
			Body:    goconfluence.Body{Storage: goconfluence.Storage{Value: "conflugen-hash:" + hash}},
			Version: &goconfluence.Version{Number: 1},
		}

		err := publishPage(api, "parent-1", "OB", "Page", "<p>content</p>", hash, false)

		assertNoError(t, err)
		assertEqual(t, 0, len(api.updated))
	})

	t.Run("dry run — ничего не создаёт", func(t *testing.T) {
		t.Parallel()

		api := newStubAPI()

		err := publishPage(api, "parent-1", "OB", "Page", "<p>x</p>", "hash", true)

		assertNoError(t, err)
		assertEqual(t, 0, len(api.created))
	})

	t.Run("dry run — ничего не обновляет", func(t *testing.T) {
		t.Parallel()

		api := newStubAPI()
		api.children["parent-1"] = &goconfluence.Search{
			Results: []goconfluence.Results{
				{ID: "page-1", Title: "Page"},
			},
		}

		err := publishPage(api, "parent-1", "OB", "Page", "<p>x</p>", "hash", true)

		assertNoError(t, err)
		assertEqual(t, 0, len(api.updated))
	})

	t.Run("ошибка API при создании", func(t *testing.T) {
		t.Parallel()

		api := newStubAPI()
		api.createErr = fmt.Errorf("api error")

		err := publishPage(api, "parent-1", "OB", "Page", "<p>x</p>", "hash", false)

		assertError(t, err)
	})
}
