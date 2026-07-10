package main

import (
	"fmt"

	goconfluence "github.com/virtomize/confluence-go-api"
)

// ancestryAPI — минимальный набор операций для резолва цепочки предков по заголовкам.
// Выделен интерфейсом, чтобы логику ResolveParentID можно было покрыть unit-тестами
// без живого Confluence.
type ancestryAPI interface {
	// FindPageByTitle ищет страницу по заголовку в пространстве (верхний уровень предков).
	FindPageByTitle(space, title string) (id string, found bool, err error)
	// FindChildByTitle ищет дочернюю страницу по заголовку под родителем parentID.
	FindChildByTitle(parentID, title string) (id string, found bool, err error)
	// CreateEmptyPage создаёт пустую страницу под parentID (пустой parentID → верхний уровень).
	CreateEmptyPage(space, parentID, title string) (id string, err error)
}

// ResolveParentID превращает цепочку заголовков предков (parents) в id финального
// родителя, под которым публикуется страница. Отсутствующие в иерархии страницы
// создаются пустыми. Верхний предок ищется в пространстве, остальные — среди детей
// предыдущего. Порядок parents — от корня к низу.
func ResolveParentID(api ancestryAPI, space string, parents []string) (string, error) {
	if len(parents) == 0 {
		return "", fmt.Errorf("ResolveParentID: пустой список parents")
	}

	parentID := ""
	for _, title := range parents {
		var (
			id    string
			found bool
			err   error
		)

		if parentID == "" {
			id, found, err = api.FindPageByTitle(space, title)
		} else {
			id, found, err = api.FindChildByTitle(parentID, title)
		}
		if err != nil {
			return "", fmt.Errorf("поиск предка %q: %w", title, err)
		}

		if !found {
			id, err = api.CreateEmptyPage(space, parentID, title)
			if err != nil {
				return "", fmt.Errorf("создание предка %q: %w", title, err)
			}
		}

		parentID = id
	}

	return parentID, nil
}

// confluenceAncestry — адаптер ancestryAPI поверх confluenceAPI (живой Confluence).
type confluenceAncestry struct {
	api confluenceAPI
}

func (c confluenceAncestry) FindPageByTitle(space, title string) (string, bool, error) {
	res, err := c.api.GetContent(goconfluence.ContentQuery{
		SpaceKey: space,
		Title:    title,
		Type:     "page",
		Limit:    1,
	})
	if err != nil {
		return "", false, err
	}
	if res == nil || len(res.Results) == 0 {
		return "", false, nil
	}
	return res.Results[0].ID, true, nil
}

func (c confluenceAncestry) FindChildByTitle(parentID, title string) (string, bool, error) {
	children, err := c.api.GetChildPages(parentID)
	if err != nil {
		return "", false, err
	}
	for i := range children.Results {
		if children.Results[i].Title == title {
			return children.Results[i].ID, true, nil
		}
	}
	return "", false, nil
}

func (c confluenceAncestry) CreateEmptyPage(space, parentID, title string) (string, error) {
	page := &goconfluence.Content{
		Type:    "page",
		Title:   title,
		Space:   &goconfluence.Space{Key: space},
		Version: &goconfluence.Version{Number: 1},
		Body: goconfluence.Body{
			Storage: goconfluence.Storage{Value: "", Representation: "storage"},
		},
	}
	if parentID != "" {
		page.Ancestors = []goconfluence.Ancestor{{ID: parentID}}
	}
	created, err := c.api.CreateContent(page)
	if err != nil {
		return "", err
	}
	return created.ID, nil
}
