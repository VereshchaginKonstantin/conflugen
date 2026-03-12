package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	goconfluence "github.com/virtomize/confluence-go-api"
)

func TestCommentHash(t *testing.T) {
	t.Parallel()

	h := commentHash("<p>test</p>")
	assertEqual(t, 64, len(h))

	h2 := commentHash("<p>test</p>")
	assertEqual(t, h, h2)

	h3 := commentHash("<p>other</p>")
	assertNotContains(t, h, h3)
}

func TestExtractSavedHashes(t *testing.T) {
	t.Parallel()

	hash := commentHash("<p>test</p>")
	body := savedMarker(hash) + "<p>rest</p>"

	hashes := extractSavedHashes(body)
	assertEqual(t, 1, len(hashes))
	assertEqual(t, hash, hashes[0])
}

func TestFetchNewInlineComments(t *testing.T) {
	t.Parallel()

	t.Run("пропускает уже сохранённые по хешу", func(t *testing.T) {
		t.Parallel()

		savedBody := "<p>уже сохранённый</p>"
		hash := commentHash(savedBody)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			if strings.Contains(r.URL.RawQuery, "location=inline") {
				json.NewEncoder(w).Encode(commentsResponse{
					Results: []commentResult{
						{
							Extensions: commentExtensions{
								InlineProperties: &inlineProperties{OriginalSelection: "новый текст"},
							},
							Body:    commentBody{Storage: goconfluence.Storage{Value: "<p>новый inline</p>"}},
							History: commentHistory{CreatedBy: commentAuthor{DisplayName: "Иван"}},
						},
						{
							Extensions: commentExtensions{
								InlineProperties: &inlineProperties{OriginalSelection: "старый текст"},
							},
							Body:    commentBody{Storage: goconfluence.Storage{Value: savedBody}},
							History: commentHistory{CreatedBy: commentAuthor{DisplayName: "Пётр"}},
						},
					},
				})
			} else {
				json.NewEncoder(w).Encode(commentsResponse{
					Results: []commentResult{
						{
							Body: commentBody{Storage: goconfluence.Storage{
								Value: savedMarker(hash) + "<p><strong>[Комментарий]</strong></p>" + savedBody,
							}},
						},
					},
				})
			}
		}))
		defer srv.Close()

		api, err := goconfluence.NewAPIWithClient(srv.URL+"/rest/api", srv.Client())
		assertNoError(t, err)

		comments, err := fetchNewInlineComments(api, srv.URL+"/rest/api", "12345")
		assertNoError(t, err)
		assertEqual(t, 1, len(comments))
		assertEqual(t, "Иван", comments[0].Author)
		assertEqual(t, "<p>новый inline</p>", comments[0].Body)
	})

	t.Run("пропускает все если все уже сохранены", func(t *testing.T) {
		t.Parallel()

		body := "<p>старый inline</p>"
		hash := commentHash(body)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			if strings.Contains(r.URL.RawQuery, "location=inline") {
				json.NewEncoder(w).Encode(commentsResponse{
					Results: []commentResult{
						{
							Body:    commentBody{Storage: goconfluence.Storage{Value: body}},
							History: commentHistory{CreatedBy: commentAuthor{DisplayName: "Иван"}},
						},
					},
				})
			} else {
				json.NewEncoder(w).Encode(commentsResponse{
					Results: []commentResult{
						{
							Body: commentBody{Storage: goconfluence.Storage{
								Value: savedMarker(hash) + "<p>wrapped</p>" + body,
							}},
						},
					},
				})
			}
		}))
		defer srv.Close()

		api, err := goconfluence.NewAPIWithClient(srv.URL+"/rest/api", srv.Client())
		assertNoError(t, err)

		comments, err := fetchNewInlineComments(api, srv.URL+"/rest/api", "12345")
		assertNoError(t, err)
		assertEqual(t, 0, len(comments))
	})

	t.Run("пустой список без комментариев", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(commentsResponse{Results: []commentResult{}})
		}))
		defer srv.Close()

		api, err := goconfluence.NewAPIWithClient(srv.URL+"/rest/api", srv.Client())
		assertNoError(t, err)

		comments, err := fetchNewInlineComments(api, srv.URL+"/rest/api", "12345")
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

	t.Run("restoreFunc создаёт комментарии с хешем в маркере", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		var postedBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			if r.Method == http.MethodGet {
				if strings.Contains(r.URL.RawQuery, "location=inline") {
					json.NewEncoder(w).Encode(commentsResponse{
						Results: []commentResult{
							{
								Extensions: commentExtensions{
									InlineProperties: &inlineProperties{OriginalSelection: "фрагмент"},
								},
								Body:    commentBody{Storage: goconfluence.Storage{Value: "<p>комментарий</p>"}},
								History: commentHistory{CreatedBy: commentAuthor{DisplayName: "Автор"}},
							},
						},
					})
				} else {
					json.NewEncoder(w).Encode(commentsResponse{Results: []commentResult{}})
				}
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

		hash := commentHash("<p>комментарий</p>")
		assertContains(t, postedBody, "conflugen-saved:"+hash)
		assertContains(t, postedBody, "<blockquote><p>фрагмент</p></blockquote>")
		assertContains(t, postedBody, "<p>комментарий</p>")
	})

	t.Run("restoreFunc ничего не делает без новых комментариев", func(t *testing.T) {
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
