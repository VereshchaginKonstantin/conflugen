package main

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	goconfluence "github.com/virtomize/confluence-go-api"
)

// confluenceAPI определяет методы Confluence, используемые conflugen
type confluenceAPI interface {
	GetContentByID(id string, query goconfluence.ContentQuery) (*goconfluence.Content, error)
	GetChildPages(id string) (*goconfluence.Search, error)
	CreateContent(content *goconfluence.Content) (*goconfluence.Content, error)
	UpdateContent(content *goconfluence.Content) (*goconfluence.Content, error)
}

var hashMacroRegex = regexp.MustCompile(`conflugen-hash:([a-f0-9]{64})`)

func extractStoredHash(html string) string {
	matches := hashMacroRegex.FindStringSubmatch(html)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// publishPage создаёт или обновляет страницу в Confluence
func publishPage(
	client confluenceAPI,
	rawAPI rawRequester,
	baseURL string,
	parentID, spaceKey, title, htmlContent, contentHash string,
	dryRun bool,
) error {
	page, err := findChildPage(client, parentID, title)
	if err != nil {
		return createPage(client, parentID, spaceKey, title, htmlContent, contentHash, dryRun)
	}

	return updatePage(client, rawAPI, baseURL, spaceKey, page.ID, title, htmlContent, contentHash, dryRun)
}

func findChildPage(client confluenceAPI, parentID, title string) (*goconfluence.Results, error) {
	children, err := client.GetChildPages(parentID)
	if err != nil {
		return nil, fmt.Errorf("GetChildPages: %w", err)
	}

	for i := range children.Results {
		if children.Results[i].Title == title {
			return &children.Results[i], nil
		}
	}

	return nil, fmt.Errorf("page not found: %s", title)
}

func createPage(
	client confluenceAPI,
	parentID, spaceKey, title, htmlContent, contentHash string,
	dryRun bool,
) error {
	if dryRun {
		log.Printf("[DRY RUN] создание: %s (parent=%s, space=%s)", title, parentID, spaceKey)
		return nil
	}

	page := &goconfluence.Content{
		Type:  "page",
		Title: title,
		Ancestors: []goconfluence.Ancestor{
			{ID: parentID},
		},
		Body: goconfluence.Body{
			Storage: goconfluence.Storage{
				Value:          annotateHTML(htmlContent, contentHash),
				Representation: "storage",
			},
		},
		Version: &goconfluence.Version{Number: 1},
		Space:   &goconfluence.Space{Key: spaceKey},
	}

	created, err := client.CreateContent(page)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "already exists") || strings.Contains(errStr, "400") {
			return fmt.Errorf(
				"страница '%s' уже существует в пространстве %s (но не под родителем %s). "+
					"Переименуйте файл или удалите дубликат в Confluence",
				title, spaceKey, parentID,
			)
		}
		return fmt.Errorf("CreateContent для %s: %w", title, err)
	}

	log.Printf("создана: %s (ID: %s)", title, created.ID)
	return nil
}

func updatePage(
	client confluenceAPI,
	rawAPI rawRequester,
	baseURL string,
	spaceKey, pageID, title, htmlContent, contentHash string,
	dryRun bool,
) error {
	currentPage, err := client.GetContentByID(pageID, goconfluence.ContentQuery{
		Expand: []string{"body.storage", "version"},
	})
	if err != nil {
		return fmt.Errorf("GetContentByID: %w", err)
	}

	storedHash := extractStoredHash(currentPage.Body.Storage.Value)
	if storedHash == contentHash {
		log.Printf("без изменений: %s", title)
		return nil
	}

	if dryRun {
		log.Printf("[DRY RUN] обновление: %s (ID: %s)", title, pageID)
		return nil
	}

	var restoreFunc func() error
	if rawAPI != nil {
		restoreFunc, err = preserveComments(rawAPI, baseURL, pageID, spaceKey)
		if err != nil {
			log.Printf("предупреждение: не удалось прочитать inline-комментарии: %v", err)
		}
	}

	version := 2
	if currentPage.Version != nil {
		version = currentPage.Version.Number + 1
	}

	updatedContent := &goconfluence.Content{
		ID:    pageID,
		Type:  "page",
		Title: title,
		Body: goconfluence.Body{
			Storage: goconfluence.Storage{
				Value:          annotateHTML(htmlContent, contentHash),
				Representation: "storage",
			},
		},
		Version: &goconfluence.Version{Number: version},
		Space:   &goconfluence.Space{Key: spaceKey},
	}

	_, err = client.UpdateContent(updatedContent)
	if err != nil {
		return fmt.Errorf("UpdateContent для %s: %w", title, err)
	}

	log.Printf("обновлена: %s (ID: %s)", title, pageID)

	if restoreFunc != nil {
		if err := restoreFunc(); err != nil {
			log.Printf("предупреждение: не удалось восстановить комментарии: %v", err)
		}
	}

	return nil
}
