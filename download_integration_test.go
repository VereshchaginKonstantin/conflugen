package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goconfluence "github.com/virtomize/confluence-go-api"
)

// fakeConfluence поднимает httptest-сервер, отвечающий как Confluence на те
// эндпоинты, которые трогает download: контент по id, метки, дочерние страницы,
// список вложений, скачивание вложения.
func fakeConfluence(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// Страница 1 ссылается на страницу 2; у неё одно вложение.
	page1 := `{
		"id": "1", "type": "page", "title": "Родитель",
		"space": {"key": "OB"},
		"version": {"number": 3},
		"ancestors": [],
		"body": {"storage": {"value": "<p>см. <a href=\"/pages/viewpage.action?pageId=2\">двойку</a></p>"}},
		"_links": {"base": "SERVER", "webui": "/display/OB/Roditel"}
	}`
	page2 := `{
		"id": "2", "type": "page", "title": "Ребёнок",
		"space": {"key": "OB"},
		"version": {"number": 1},
		"ancestors": [{"id": "1"}],
		"body": {"storage": {"value": "<p>лист</p>"}},
		"_links": {"base": "SERVER", "webui": "/display/OB/Rebenok"}
	}`

	mux.HandleFunc("/rest/api/content/1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.ReplaceAll(page1, "SERVER", "http://"+r.Host))
	})
	mux.HandleFunc("/rest/api/content/2", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.ReplaceAll(page2, "SERVER", "http://"+r.Host))
	})

	mux.HandleFunc("/rest/api/content/1/label", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"results":[{"prefix":"global","name":"arch"}]}`)
	})
	mux.HandleFunc("/rest/api/content/2/label", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"results":[]}`)
	})

	mux.HandleFunc("/rest/api/content/1/child/page", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"results":[{"id":"2"}],"size":1,"limit":25}`)
	})
	mux.HandleFunc("/rest/api/content/2/child/page", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"results":[],"size":0,"limit":25}`)
	})

	mux.HandleFunc("/rest/api/content/1/child/attachment", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"results":[{"id":"att1","title":"pic.png","_links":{"download":"/download/attachments/1/pic.png?version=1"}}],"size":1,"limit":50}`)
	})
	mux.HandleFunc("/rest/api/content/2/child/attachment", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"results":[],"size":0,"limit":50}`)
	})

	mux.HandleFunc("/download/attachments/1/pic.png", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("PNG-BYTES"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadEndToEnd(t *testing.T) {
	srv := fakeConfluence(t)
	dir := t.TempDir()

	// goconfluence.API реализует оба наших интерфейса — confluenceAPI и
	// rawRequester, — поэтому передаётся дважды. Это не опечатка.
	api, err := goconfluence.NewAPI(srv.URL+"/rest/api", "", "token")
	if err != nil {
		t.Fatalf("клиент: %v", err)
	}

	src := newPageSource(api, api, srv.URL+"/rest/api")
	store := newPageStore(dir)
	host := strings.TrimPrefix(srv.URL, "http://")
	c := newCrawler(src, store, host, false)

	if err := c.Crawl(context.Background(), []string{"1"}); err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if err := store.FlushIndex(); err != nil {
		t.Fatal(err)
	}

	body1, err := os.ReadFile(filepath.Join(dir, "1-roditel", "page.xhtml"))
	if err != nil {
		t.Fatalf("page.xhtml страницы 1: %v", err)
	}
	if !strings.Contains(string(body1), "двойку") {
		t.Errorf("тело страницы 1 = %q", body1)
	}

	if _, err := os.Stat(filepath.Join(dir, "2-rebyonok", "page.json")); err != nil {
		t.Errorf("страница 2 не выгружена: %v", err)
	}

	att, err := os.ReadFile(filepath.Join(dir, "1-roditel", "attachments", "pic.png"))
	if err != nil || string(att) != "PNG-BYTES" {
		t.Errorf("вложение = %q, err = %v", att, err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "1-roditel", "page.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m pageMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Version != 3 {
		t.Errorf("версия = %d, хотим 3", m.Version)
	}
	if len(m.Labels) != 1 || m.Labels[0] != "arch" {
		t.Errorf("метки = %v", m.Labels)
	}
	if len(m.Attachments) != 1 || m.Attachments[0] != "pic.png" {
		t.Errorf("вложения в page.json = %v", m.Attachments)
	}

	idxRaw, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var idx dumpIndex
	if err := json.Unmarshal(idxRaw, &idx); err != nil {
		t.Fatal(err)
	}
	if len(idx.Pages) != 2 {
		t.Errorf("страниц в index.json = %d, хотим 2", len(idx.Pages))
	}
	foundEdge := false
	for _, e := range idx.Edges {
		if e.From == "1" && e.To == "2" {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Errorf("нет ребра 1→2 в index.json: %+v", idx.Edges)
	}
}

func TestDownloadResumeSkipsSecondRun(t *testing.T) {
	srv := fakeConfluence(t)
	dir := t.TempDir()

	api, err := goconfluence.NewAPI(srv.URL+"/rest/api", "", "token")
	if err != nil {
		t.Fatal(err)
	}
	host := strings.TrimPrefix(srv.URL, "http://")

	run := func() {
		src := newPageSource(api, api, srv.URL+"/rest/api")
		store := newPageStore(dir)
		c := newCrawler(src, store, host, false)
		if err := c.Crawl(context.Background(), []string{"1"}); err != nil {
			t.Fatalf("Crawl: %v", err)
		}
		if err := store.FlushIndex(); err != nil {
			t.Fatal(err)
		}
	}

	run()

	// Портим содержимое вложения: если второй прогон его перекачает — resume не работает.
	attPath := filepath.Join(dir, "1-roditel", "attachments", "pic.png")
	if err := os.WriteFile(attPath, []byte("СТАРОЕ"), 0o644); err != nil {
		t.Fatal(err)
	}

	run()

	got, err := os.ReadFile(attPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "СТАРОЕ" {
		t.Errorf("resume должен пропустить скачивание вложения, но файл перезаписан: %q", got)
	}
}
