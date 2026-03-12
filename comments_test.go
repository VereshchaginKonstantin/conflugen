package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	goconfluence "github.com/virtomize/confluence-go-api"
)

func TestFetchAllComments(t *testing.T) {
	t.Parallel()

	t.Run("возвращает все комментарии с ID и inline-данными", func(t *testing.T) {
		t.Parallel()

		response := commentsResponse{
			Results: []commentResult{
				{
					ID: "100",
					Extensions: commentExtensions{
						InlineProperties: &inlineProperties{OriginalSelection: "выделенный текст"},
					},
					Body:    commentBody{Storage: goconfluence.Storage{Value: "<p>inline комментарий</p>"}},
					History: commentHistory{CreatedBy: commentAuthor{DisplayName: "Иван Иванов"}},
				},
				{
					ID:         "200",
					Extensions: commentExtensions{InlineProperties: nil},
					Body:       commentBody{Storage: goconfluence.Storage{Value: "<p>обычный комментарий</p>"}},
					History:    commentHistory{CreatedBy: commentAuthor{DisplayName: "Пётр Петров"}},
				},
			},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assertEqual(t, "/rest/api/content/12345/child/comment", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer srv.Close()

		api, err := goconfluence.NewAPIWithClient(srv.URL+"/rest/api", srv.Client())
		assertNoError(t, err)

		comments, err := fetchAllComments(api, srv.URL+"/rest/api", "12345")
		assertNoError(t, err)
		assertEqual(t, 2, len(comments))

		assertEqual(t, "100", comments[0].ID)
		assertEqual(t, "Иван Иванов", comments[0].Author)
		assertEqual(t, "выделенный текст", comments[0].OriginalSelection)

		assertEqual(t, "200", comments[1].ID)
		assertEqual(t, "Пётр Петров", comments[1].Author)
		assertEqual(t, "", comments[1].OriginalSelection)
	})

	t.Run("пустой список", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(commentsResponse{Results: []commentResult{}})
		}))
		defer srv.Close()

		api, err := goconfluence.NewAPIWithClient(srv.URL+"/rest/api", srv.Client())
		assertNoError(t, err)

		comments, err := fetchAllComments(api, srv.URL+"/rest/api", "12345")
		assertNoError(t, err)
		assertEqual(t, 0, len(comments))
	})
}

func TestDeleteComment(t *testing.T) {
	t.Parallel()

	t.Run("отправляет DELETE запрос", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assertEqual(t, http.MethodDelete, r.Method)
			assertEqual(t, "/rest/api/content/100", r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		api, err := goconfluence.NewAPIWithClient(srv.URL+"/rest/api", srv.Client())
		assertNoError(t, err)

		err = deleteComment(api, srv.URL+"/rest/api", "100")
		assertNoError(t, err)
	})
}

func TestCreatePageComment(t *testing.T) {
	t.Parallel()

	t.Run("отправляет корректный JSON", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assertEqual(t, http.MethodPost, r.Method)
			assertEqual(t, "/rest/api/content/", r.URL.Path)
			assertEqual(t, "application/json", r.Header.Get("Content-Type"))

			body, err := io.ReadAll(r.Body)
			assertNoError(t, err)

			var req createCommentRequest
			err = json.Unmarshal(body, &req)
			assertNoError(t, err)

			assertEqual(t, "comment", req.Type)
			assertEqual(t, "12345", req.Container.ID)
			assertEqual(t, "page", req.Container.Type)
			assertEqual(t, "OB", req.Space.Key)
			assertEqual(t, "storage", req.Body.Storage.Representation)
			assertContains(t, req.Body.Storage.Value, "тестовый комментарий")

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"99999","type":"comment"}`))
		}))
		defer srv.Close()

		api, err := goconfluence.NewAPIWithClient(srv.URL+"/rest/api", srv.Client())
		assertNoError(t, err)

		err = createPageComment(api, srv.URL+"/rest/api", "12345", "OB", "<p>тестовый комментарий</p>")
		assertNoError(t, err)
	})
}

func TestFormatCommentBody(t *testing.T) {
	t.Parallel()

	t.Run("inline комментарий с цитатой", func(t *testing.T) {
		t.Parallel()

		result := formatCommentBody(commentData{
			Author:            "Иван",
			Body:              "<p>мой комментарий</p>",
			OriginalSelection: "выделенный текст",
		})
		assertContains(t, result, "Иван")
		assertContains(t, result, "<blockquote><p>выделенный текст</p></blockquote>")
		assertContains(t, result, "<p>мой комментарий</p>")
		assertContains(t, result, "перенесён conflugen")
	})

	t.Run("обычный комментарий без цитаты", func(t *testing.T) {
		t.Parallel()

		result := formatCommentBody(commentData{
			Author: "Пётр",
			Body:   "<p>обычный</p>",
		})
		assertContains(t, result, "Пётр")
		assertContains(t, result, "<p>обычный</p>")
		assertNotContains(t, result, "blockquote")
	})
}

func TestPreserveComments(t *testing.T) {
	t.Parallel()

	t.Run("удаляет и пересоздаёт комментарии", func(t *testing.T) {
		t.Parallel()

		var deletedIDs []string
		var createdCount int

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch r.Method {
			case http.MethodGet:
				response := commentsResponse{
					Results: []commentResult{
						{
							ID: "100",
							Extensions: commentExtensions{
								InlineProperties: &inlineProperties{OriginalSelection: "текст"},
							},
							Body:    commentBody{Storage: goconfluence.Storage{Value: "<p>inline</p>"}},
							History: commentHistory{CreatedBy: commentAuthor{DisplayName: "Автор"}},
						},
						{
							ID:         "200",
							Extensions: commentExtensions{},
							Body:       commentBody{Storage: goconfluence.Storage{Value: "<p>обычный</p>"}},
							History:    commentHistory{CreatedBy: commentAuthor{DisplayName: "Другой"}},
						},
					},
				}
				json.NewEncoder(w).Encode(response)

			case http.MethodDelete:
				deletedIDs = append(deletedIDs, r.URL.Path)
				w.WriteHeader(http.StatusNoContent)

			case http.MethodPost:
				createdCount++
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"id":"99999","type":"comment"}`))
			}
		}))
		defer srv.Close()

		api, err := goconfluence.NewAPIWithClient(srv.URL+"/rest/api", srv.Client())
		assertNoError(t, err)

		restoreFunc, err := preserveComments(api, srv.URL+"/rest/api", "12345", "OB")
		assertNoError(t, err)

		// preserveComments уже удалила комментарии
		assertEqual(t, 2, len(deletedIDs))
		assertContains(t, deletedIDs[0], "/100")
		assertContains(t, deletedIDs[1], "/200")

		err = restoreFunc()
		assertNoError(t, err)
		assertEqual(t, 2, createdCount)
	})

	t.Run("ничего не делает без комментариев", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(commentsResponse{Results: []commentResult{}})
		}))
		defer srv.Close()

		api, err := goconfluence.NewAPIWithClient(srv.URL+"/rest/api", srv.Client())
		assertNoError(t, err)

		restoreFunc, err := preserveComments(api, srv.URL+"/rest/api", "12345", "OB")
		assertNoError(t, err)

		err = restoreFunc()
		assertNoError(t, err)
	})
}
