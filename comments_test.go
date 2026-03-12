package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	goconfluence "github.com/virtomize/confluence-go-api"
)

func TestFetchInlineComments(t *testing.T) {
	t.Parallel()

	t.Run("возвращает все комментарии кроме помеченных маркером", func(t *testing.T) {
		t.Parallel()

		response := commentsResponse{
			Results: []commentResult{
				{
					Extensions: commentExtensions{
						InlineProperties: &inlineProperties{OriginalSelection: "выделенный текст"},
					},
					Body:    commentBody{Storage: goconfluence.Storage{Value: "<p>inline комментарий</p>"}},
					History: commentHistory{CreatedBy: commentAuthor{DisplayName: "Иван Иванов"}},
				},
				{
					Extensions: commentExtensions{InlineProperties: nil},
					Body:       commentBody{Storage: goconfluence.Storage{Value: "<p>обычный комментарий</p>"}},
					History:    commentHistory{CreatedBy: commentAuthor{DisplayName: "Пётр Петров"}},
				},
				{
					Extensions: commentExtensions{},
					Body: commentBody{Storage: goconfluence.Storage{
						Value: "<!-- conflugen-saved --><p><strong>[Комментарий от Кто-то, перенесён conflugen]:</strong></p><p>старый</p>",
					}},
					History: commentHistory{CreatedBy: commentAuthor{DisplayName: "Кто-то"}},
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

		comments, err := fetchInlineComments(api, srv.URL+"/rest/api", "12345")
		assertNoError(t, err)
		assertEqual(t, 2, len(comments))
		assertEqual(t, "Иван Иванов", comments[0].Author)
		assertEqual(t, "выделенный текст", comments[0].OriginalSelection)
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

		comments, err := fetchInlineComments(api, srv.URL+"/rest/api", "12345")
		assertNoError(t, err)
		assertEqual(t, 0, len(comments))
	})

	t.Run("только маркированные — пустой результат", func(t *testing.T) {
		t.Parallel()

		response := commentsResponse{
			Results: []commentResult{
				{
					Body: commentBody{Storage: goconfluence.Storage{
						Value: "<!-- conflugen-saved --><p>ранее сохранённый</p>",
					}},
					History: commentHistory{CreatedBy: commentAuthor{DisplayName: "Bot"}},
				},
			},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer srv.Close()

		api, err := goconfluence.NewAPIWithClient(srv.URL+"/rest/api", srv.Client())
		assertNoError(t, err)

		comments, err := fetchInlineComments(api, srv.URL+"/rest/api", "12345")
		assertNoError(t, err)
		assertEqual(t, 0, len(comments))
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

func TestPreserveComments(t *testing.T) {
	t.Parallel()

	t.Run("restoreFunc создаёт комментарии с маркером и цитатой", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		var postedBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			if r.Method == http.MethodGet {
				response := commentsResponse{
					Results: []commentResult{
						{
							Extensions: commentExtensions{
								InlineProperties: &inlineProperties{OriginalSelection: "выделенный фрагмент"},
							},
							Body:    commentBody{Storage: goconfluence.Storage{Value: "<p>комментарий 1</p>"}},
							History: commentHistory{CreatedBy: commentAuthor{DisplayName: "Автор"}},
						},
					},
				}
				json.NewEncoder(w).Encode(response)
				return
			}

			if r.Method == http.MethodPost {
				callCount++
				body, _ := io.ReadAll(r.Body)
				var req createCommentRequest
				json.Unmarshal(body, &req)
				postedBody = req.Body.Storage.Value
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"id":"99999","type":"comment"}`))
				return
			}
		}))
		defer srv.Close()

		api, err := goconfluence.NewAPIWithClient(srv.URL+"/rest/api", srv.Client())
		assertNoError(t, err)

		restoreFunc, err := preserveComments(api, srv.URL+"/rest/api", "12345", "OB")
		assertNoError(t, err)

		err = restoreFunc()
		assertNoError(t, err)
		assertEqual(t, 1, callCount)
		assertContains(t, postedBody, "<!-- conflugen-saved -->")
		assertContains(t, postedBody, "<blockquote><p>выделенный фрагмент</p></blockquote>")
		assertContains(t, postedBody, "<p>комментарий 1</p>")
		assertContains(t, postedBody, "Автор")
	})

	t.Run("restoreFunc ничего не делает без комментариев", func(t *testing.T) {
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
