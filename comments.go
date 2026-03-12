package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	goconfluence "github.com/virtomize/confluence-go-api"
)

var savedHashRegex = regexp.MustCompile(`conflugen-saved:([a-f0-9]{64})`)

// rawRequester выполняет HTTP-запросы с авторизацией Confluence
type rawRequester interface {
	Request(req *http.Request) ([]byte, error)
}

// commentData — данные комментария со страницы
type commentData struct {
	Author            string
	Body              string
	OriginalSelection string
}

// commentHash вычисляет sha256 хеш тела комментария
func commentHash(body string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
}

// savedMarker создаёт текстовый маркер с хешем
func savedMarker(hash string) string {
	return fmt.Sprintf("conflugen-saved:%s", hash)
}

// extractSavedHashes извлекает все хеши conflugen-saved из тела комментария
func extractSavedHashes(body string) []string {
	matches := savedHashRegex.FindAllStringSubmatch(body, -1)
	hashes := make([]string, 0, len(matches))
	for _, m := range matches {
		hashes = append(hashes, m[1])
	}
	return hashes
}

// commentsResponse — структура ответа Confluence API для комментариев
type commentsResponse struct {
	Results []commentResult `json:"results"`
}

type commentResult struct {
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

// fetchComments получает комментарии страницы по URL
func fetchComments(requester rawRequester, url string) ([]commentResult, error) {
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

	return resp.Results, nil
}

// fetchNewInlineComments получает inline-комментарии, которые ещё не были сохранены conflugen
func fetchNewInlineComments(requester rawRequester, baseURL, pageID string) ([]commentData, error) {
	// Получаем все комментарии и собираем хеши уже сохранённых
	allURL := baseURL + "/content/" + pageID + "/child/comment?expand=body.storage"
	allResults, err := fetchComments(requester, allURL)
	if err != nil {
		return nil, fmt.Errorf("чтение всех комментариев: %w", err)
	}

	savedHashes := make(map[string]bool)
	for _, r := range allResults {
		for _, h := range extractSavedHashes(r.Body.Storage.Value) {
			savedHashes[h] = true
		}
	}

	log.Printf("найдено %d сохранённых хешей комментариев", len(savedHashes))

	// Получаем inline-комментарии
	inlineURL := baseURL + "/content/" + pageID + "/child/comment?location=inline&expand=body.storage,extensions.inlineProperties,history"
	inlineResults, err := fetchComments(requester, inlineURL)
	if err != nil {
		return nil, fmt.Errorf("чтение inline-комментариев: %w", err)
	}

	var comments []commentData
	for _, r := range inlineResults {
		if strings.Contains(r.Body.Storage.Value, "conflugen-saved:") {
			continue
		}
		h := commentHash(r.Body.Storage.Value)
		if savedHashes[h] {
			continue
		}
		comments = append(comments, commentData{
			Author:            r.History.CreatedBy.DisplayName,
			Body:              r.Body.Storage.Value,
			OriginalSelection: inlineSelection(r.Extensions.InlineProperties),
		})
	}

	return comments, nil
}

func inlineSelection(props *inlineProperties) string {
	if props == nil {
		return ""
	}
	return props.OriginalSelection
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

// preserveComments читает inline-комментарии и возвращает функцию для их восстановления как обычных комментариев
func preserveComments(requester rawRequester, baseURL, pageID, spaceKey string) (restoreFunc func() error, err error) {
	comments, err := fetchNewInlineComments(requester, baseURL, pageID)
	if err != nil {
		return nil, fmt.Errorf("чтение inline-комментариев: %w", err)
	}

	if len(comments) == 0 {
		return func() error { return nil }, nil
	}

	log.Printf("найдено %d inline-комментариев для сохранения", len(comments))

	return func() error {
		for _, c := range comments {
			hash := commentHash(c.Body)
			marker := savedMarker(hash)
			quote := ""
			if c.OriginalSelection != "" {
				quote = fmt.Sprintf("<blockquote><p>%s</p></blockquote>", c.OriginalSelection)
			}
			text := fmt.Sprintf("<p><strong>[Комментарий от %s, перенесён conflugen]:</strong></p>%s%s<p><sub>%s</sub></p>",
				c.Author, quote, c.Body, marker)
			if err := createPageComment(requester, baseURL, pageID, spaceKey, text); err != nil {
				return fmt.Errorf("восстановление комментария: %w", err)
			}
		}
		log.Printf("восстановлено %d комментариев", len(comments))
		return nil
	}, nil
}
