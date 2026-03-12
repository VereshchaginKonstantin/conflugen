package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	goconfluence "github.com/virtomize/confluence-go-api"
)

// rawRequester выполняет HTTP-запросы с авторизацией Confluence
type rawRequester interface {
	Request(req *http.Request) ([]byte, error)
}

// commentData — сохранённые данные комментария
type commentData struct {
	ID                string
	Author            string
	Body              string
	OriginalSelection string // непустой для inline-комментариев
}

// commentsResponse — структура ответа Confluence API для комментариев
type commentsResponse struct {
	Results []commentResult `json:"results"`
}

type commentResult struct {
	ID         string            `json:"id"`
	Extensions commentExtensions `json:"extensions"`
	Body       commentBody       `json:"body"`
	History    commentHistory    `json:"history"`
}

type commentExtensions struct {
	InlineProperties *inlineProperties `json:"inlineProperties"`
}

type inlineProperties struct {
	OriginalSelection string `json:"originalSelection"`
}

type commentBody struct {
	Storage goconfluence.Storage `json:"storage"`
}

type commentHistory struct {
	CreatedBy commentAuthor `json:"createdBy"`
}

type commentAuthor struct {
	DisplayName string `json:"displayName"`
}

// createCommentRequest — тело POST-запроса для создания комментария
type createCommentRequest struct {
	Type      string                 `json:"type"`
	Container createCommentContainer `json:"container"`
	Space     createCommentSpace     `json:"space"`
	Body      createCommentBody      `json:"body"`
}

type createCommentContainer struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type createCommentSpace struct {
	Key string `json:"key"`
}

type createCommentBody struct {
	Storage goconfluence.Storage `json:"storage"`
}

// fetchAllComments получает все комментарии страницы
func fetchAllComments(requester rawRequester, baseURL, pageID string) ([]commentData, error) {
	url := baseURL + "/content/" + pageID + "/child/comment?expand=body.storage,extensions.inlineProperties,history"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("создание запроса комментариев: %w", err)
	}

	body, err := requester.Request(req)
	if err != nil {
		return nil, fmt.Errorf("запрос комментариев: %w", err)
	}

	var resp commentsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("парсинг комментариев: %w", err)
	}

	var comments []commentData
	for _, r := range resp.Results {
		c := commentData{
			ID:     r.ID,
			Author: r.History.CreatedBy.DisplayName,
			Body:   r.Body.Storage.Value,
		}
		if r.Extensions.InlineProperties != nil {
			c.OriginalSelection = r.Extensions.InlineProperties.OriginalSelection
		}
		comments = append(comments, c)
	}

	return comments, nil
}

// deleteComment удаляет комментарий по ID
func deleteComment(requester rawRequester, baseURL, commentID string) error {
	url := baseURL + "/content/" + commentID

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("создание запроса удаления комментария %s: %w", commentID, err)
	}

	_, err = requester.Request(req)
	if err != nil {
		return fmt.Errorf("удаление комментария %s: %w", commentID, err)
	}

	return nil
}

// createPageComment создаёт обычный комментарий к странице
func createPageComment(requester rawRequester, baseURL, pageID, spaceKey, body string) error {
	payload := createCommentRequest{
		Type:      "comment",
		Container: createCommentContainer{ID: pageID, Type: "page"},
		Space:     createCommentSpace{Key: spaceKey},
		Body: createCommentBody{
			Storage: goconfluence.Storage{
				Value:          body,
				Representation: "storage",
			},
		},
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("сериализация комментария: %w", err)
	}

	url := baseURL + "/content/"

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("создание запроса создания комментария: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	_, err = requester.Request(req)
	if err != nil {
		return fmt.Errorf("создание комментария: %w", err)
	}

	return nil
}

// formatCommentBody форматирует тело комментария для пересоздания
func formatCommentBody(c commentData) string {
	if c.OriginalSelection != "" {
		return fmt.Sprintf(
			"<p><strong>[Комментарий от %s, перенесён conflugen]:</strong></p><blockquote><p>%s</p></blockquote>%s",
			c.Author, c.OriginalSelection, c.Body,
		)
	}
	return fmt.Sprintf(
		"<p><strong>[Комментарий от %s, перенесён conflugen]:</strong></p>%s",
		c.Author, c.Body,
	)
}

// preserveComments сохраняет все комментарии, удаляет их, и возвращает функцию для пересоздания
func preserveComments(requester rawRequester, baseURL, pageID, spaceKey string) (restoreFunc func() error, err error) {
	comments, err := fetchAllComments(requester, baseURL, pageID)
	if err != nil {
		return nil, fmt.Errorf("чтение комментариев: %w", err)
	}

	if len(comments) == 0 {
		return func() error { return nil }, nil
	}

	log.Printf("найдено %d комментариев для сохранения", len(comments))

	for _, c := range comments {
		if err := deleteComment(requester, baseURL, c.ID); err != nil {
			return nil, fmt.Errorf("удаление комментария перед обновлением: %w", err)
		}
	}

	log.Printf("удалено %d комментариев", len(comments))

	return func() error {
		for _, c := range comments {
			text := formatCommentBody(c)
			if err := createPageComment(requester, baseURL, pageID, spaceKey, text); err != nil {
				return fmt.Errorf("восстановление комментария: %w", err)
			}
		}
		log.Printf("восстановлено %d комментариев", len(comments))
		return nil
	}, nil
}
