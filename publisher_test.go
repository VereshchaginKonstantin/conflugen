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

	labels      map[string][]string // pageID -> текущие метки
	addedLabels map[string][]string // pageID -> что добавляли
	delLabels   map[string][]string // pageID -> что удаляли

	createErr error
	getErr    error
	childErr  error
	updateErr error

	// Счётчики и точечные ошибки — для тестов download-краулера: он проверяет,
	// что каждая страница запрашивается ровно один раз и что упавшая страница
	// не опрашивается повторно.
	getContentCalls int
	getByIDCalls    map[string]int
	errByID         map[string]error
	byTitle         map[string]string // "SPACE/Title" -> pageID
}

func newStubAPI() *stubAPI {
	return &stubAPI{
		pages:       make(map[string]*goconfluence.Content),
		children:    make(map[string]*goconfluence.Search),
		labels:       make(map[string][]string),
		addedLabels:  make(map[string][]string),
		delLabels:    make(map[string][]string),
		getByIDCalls: make(map[string]int),
		byTitle:      make(map[string]string),
	}
}

func (s *stubAPI) GetLabels(id string) (*goconfluence.Labels, error) {
	out := &goconfluence.Labels{}
	for _, name := range s.labels[id] {
		out.Labels = append(out.Labels, goconfluence.Label{Prefix: "global", Name: name})
	}
	return out, nil
}

func (s *stubAPI) AddLabels(id string, labels *[]goconfluence.Label) (*goconfluence.Labels, error) {
	if labels == nil {
		return &goconfluence.Labels{}, nil
	}
	for _, l := range *labels {
		s.labels[id] = append(s.labels[id], l.Name)
		s.addedLabels[id] = append(s.addedLabels[id], l.Name)
	}
	return &goconfluence.Labels{}, nil
}

func (s *stubAPI) DeleteLabel(id, name string) (*goconfluence.Labels, error) {
	kept := s.labels[id][:0]
	for _, n := range s.labels[id] {
		if n != name {
			kept = append(kept, n)
		}
	}
	s.labels[id] = kept
	s.delLabels[id] = append(s.delLabels[id], name)
	return &goconfluence.Labels{}, nil
}

func (s *stubAPI) GetContentByID(id string, _ goconfluence.ContentQuery) (*goconfluence.Content, error) {
	s.getByIDCalls[id]++
	if err, ok := s.errByID[id]; ok {
		return nil, err
	}
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

func (s *stubAPI) GetContent(query goconfluence.ContentQuery) (*goconfluence.ContentSearch, error) {
	s.getContentCalls++
	if s.getErr != nil {
		return nil, s.getErr
	}
	if id, ok := s.byTitle[query.SpaceKey+"/"+query.Title]; ok {
		return &goconfluence.ContentSearch{
			Results: []goconfluence.Content{{ID: id, Title: query.Title}},
		}, nil
	}
	return &goconfluence.ContentSearch{Results: []goconfluence.Content{}}, nil
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
	s.created = append(s.created, content)
	return &goconfluence.Content{
		ID:    "new-" + content.Title,
		Title: content.Title,
		Type:  content.Type,
	}, nil
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

		_, err := publishPage(api, nil, "", "", "parent-1", "OB", "New Page", "<p>content</p>", "hash123", "", false)

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

		_, err := publishPage(api, nil, "", "", "parent-1", "OB", "Existing", "<p>new</p>", "newhash", "", false)

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

		_, err := publishPage(api, nil, "", "", "parent-1", "OB", "Page", "<p>content</p>", hash, "", false)

		assertNoError(t, err)
		assertEqual(t, 0, len(api.updated))
	})

	t.Run("dry run — ничего не создаёт", func(t *testing.T) {
		t.Parallel()

		api := newStubAPI()

		_, err := publishPage(api, nil, "", "", "parent-1", "OB", "Page", "<p>x</p>", "hash", "", true)

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

		_, err := publishPage(api, nil, "", "", "parent-1", "OB", "Page", "<p>x</p>", "hash", "", true)

		assertNoError(t, err)
		assertEqual(t, 0, len(api.updated))
	})

	t.Run("page-id — обновление напрямую, минуя поиск родителя", func(t *testing.T) {
		t.Parallel()

		api := newStubAPI()
		// Родитель не листится (как 404 на child/page в проде) — без page-id
		// это привело бы к попытке создания. С page-id поиск пропускается.
		api.childErr = fmt.Errorf("404 no parent or not permitted")
		api.pages["1007007496"] = &goconfluence.Content{
			ID:      "1007007496",
			Title:   "[WIP] Команда Рисков B2C",
			Body:    goconfluence.Body{Storage: goconfluence.Storage{Value: "old"}},
			Version: &goconfluence.Version{Number: 3},
		}

		_, err := publishPage(api, nil, "", "1007007496", "", "riskdev", "[WIP] Команда Рисков B2C", "<p>new</p>", "newhash", "", false)

		assertNoError(t, err)
		assertEqual(t, 0, len(api.created))
		assertEqual(t, 1, len(api.updated))
		assertEqual(t, "1007007496", api.updated[0].ID)
	})

	t.Run("page-id + несовпадающий title — ошибка, без обновления", func(t *testing.T) {
		t.Parallel()

		api := newStubAPI()
		api.pages["1007007496"] = &goconfluence.Content{
			ID:      "1007007496",
			Title:   "[WIP] Команда Рисков B2C",
			Body:    goconfluence.Body{Storage: goconfluence.Storage{Value: "old"}},
			Version: &goconfluence.Version{Number: 3},
		}

		// title не совпадает с реальным заголовком страницы — защита от
		// указания page-id не на ту страницу.
		_, err := publishPage(api, nil, "", "1007007496", "", "riskdev", "Совсем другая страница", "<p>new</p>", "newhash", "", false)

		assertError(t, err)
		assertEqual(t, 0, len(api.updated))
	})

	t.Run("page-id + совпадающий title — обновляет", func(t *testing.T) {
		t.Parallel()

		api := newStubAPI()
		api.pages["1007007496"] = &goconfluence.Content{
			ID:      "1007007496",
			Title:   "[WIP] Команда Рисков B2C",
			Body:    goconfluence.Body{Storage: goconfluence.Storage{Value: "old"}},
			Version: &goconfluence.Version{Number: 3},
		}

		_, err := publishPage(api, nil, "", "1007007496", "", "riskdev", "[WIP] Команда Рисков B2C", "<p>new</p>", "newhash", "", false)

		assertNoError(t, err)
		assertEqual(t, 1, len(api.updated))
	})

	t.Run("page-id без title — сохраняет существующий заголовок", func(t *testing.T) {
		t.Parallel()

		api := newStubAPI()
		api.pages["1007007496"] = &goconfluence.Content{
			ID:      "1007007496",
			Title:   "[WIP] Команда Рисков B2C",
			Body:    goconfluence.Body{Storage: goconfluence.Storage{Value: "old"}},
			Version: &goconfluence.Version{Number: 3},
		}

		// title пустой — должен подтянуться из текущей страницы, не переименовать.
		_, err := publishPage(api, nil, "", "1007007496", "", "riskdev", "", "<p>new</p>", "newhash", "", false)

		assertNoError(t, err)
		assertEqual(t, 1, len(api.updated))
		assertEqual(t, "[WIP] Команда Рисков B2C", api.updated[0].Title)
	})

	t.Run("ошибка API при создании", func(t *testing.T) {
		t.Parallel()

		api := newStubAPI()
		api.createErr = fmt.Errorf("api error")

		_, err := publishPage(api, nil, "", "", "parent-1", "OB", "Page", "<p>x</p>", "hash", "", false)

		assertError(t, err)
	})
}
