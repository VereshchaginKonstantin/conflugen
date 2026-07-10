package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goconfluence "github.com/virtomize/confluence-go-api"
)

// crawlEnv собирает краулер поверх стабов и временной папки.
func crawlEnv(t *testing.T, api *stubAPI, raw rawRequester) (*crawler, string) {
	t.Helper()
	dir := t.TempDir()
	if raw == nil {
		raw = &fakeRaw{responses: map[string][]byte{}}
	}
	src := newPageSource(api, raw, "https://conf.example.com/rest/api")
	store := newPageStore(dir)
	return newCrawler(src, store, "conf.example.com", false), dir
}

// emptyAttachments — стаб raw, отвечающий «вложений нет» на любой запрос списка.
type emptyAttachments struct{}

func (emptyAttachments) Request(_ *http.Request) ([]byte, error) {
	return []byte(`{"results":[],"size":0,"limit":50}`), nil
}

func TestCrawlerCycleTerminates(t *testing.T) {
	api := newStubAPI()
	// A ссылается на B, B ссылается обратно на A — цикл.
	api.pages["A"] = &goconfluence.Content{
		ID: "A", Title: "Page A",
		Body:    goconfluence.Body{Storage: goconfluence.Storage{Value: `<a href="/pages/viewpage.action?pageId=B">B</a>`}},
		Version: &goconfluence.Version{Number: 1},
		Space:   &goconfluence.Space{Key: "OB"},
	}
	api.pages["B"] = &goconfluence.Content{
		ID: "B", Title: "Page B",
		Body:    goconfluence.Body{Storage: goconfluence.Storage{Value: `<a href="/pages/viewpage.action?pageId=A">A</a>`}},
		Version: &goconfluence.Version{Number: 1},
		Space:   &goconfluence.Space{Key: "OB"},
	}

	c, dir := crawlEnv(t, api, emptyAttachments{})

	if err := c.Crawl(context.Background(), []string{"A"}); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	if api.getByIDCalls["A"] != 1 || api.getByIDCalls["B"] != 1 {
		t.Errorf("каждая страница должна быть запрошена ровно один раз, получили A=%d B=%d",
			api.getByIDCalls["A"], api.getByIDCalls["B"])
	}
	for _, name := range []string{"A-page-a", "B-page-b"} {
		if _, err := os.Stat(filepath.Join(dir, name, "page.json")); err != nil {
			t.Errorf("нет page.json для %s: %v", name, err)
		}
	}
}

func TestCrawlerFailedPageNotRetried(t *testing.T) {
	api := newStubAPI()
	// Корень ссылается на X дважды. X отдаёт 403.
	api.pages["root"] = &goconfluence.Content{
		ID: "root", Title: "Root",
		Body: goconfluence.Body{Storage: goconfluence.Storage{Value: `
			<a href="/pages/viewpage.action?pageId=X">раз</a>
			<a href="/pages/viewpage.action?pageId=X">два</a>`}},
		Version: &goconfluence.Version{Number: 1},
		Space:   &goconfluence.Space{Key: "OB"},
	}
	api.errByID = map[string]error{"X": errors.New("403 Forbidden")}

	c, dir := crawlEnv(t, api, emptyAttachments{})

	if err := c.Crawl(context.Background(), []string{"root"}); err != nil {
		t.Fatalf("Crawl не должен падать из-за 403 на одной странице: %v", err)
	}

	if api.getByIDCalls["X"] != 1 {
		t.Errorf("упавшая страница запрошена %d раз, хотим ровно 1", api.getByIDCalls["X"])
	}

	raw, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatalf("index.json: %v", err)
	}
	if !strings.Contains(string(raw), "403 Forbidden") {
		t.Errorf("ошибка страницы X не попала в index.json:\n%s", raw)
	}
}

func TestCrawlerAuthErrorIsFatal(t *testing.T) {
	api := newStubAPI()
	api.errByID = map[string]error{"A": errors.New("authentication failed")}

	c, _ := crawlEnv(t, api, emptyAttachments{})

	err := c.Crawl(context.Background(), []string{"A"})
	if !errors.Is(err, errAuth) {
		t.Errorf("401 должен ронять весь обход, получили %v", err)
	}
}

func TestCrawlerFollowsChildren(t *testing.T) {
	api := newStubAPI()
	api.pages["p"] = &goconfluence.Content{
		ID: "p", Title: "Parent",
		Body:    goconfluence.Body{Storage: goconfluence.Storage{Value: ``}},
		Version: &goconfluence.Version{Number: 1},
		Space:   &goconfluence.Space{Key: "OB"},
	}
	api.pages["c"] = &goconfluence.Content{
		ID: "c", Title: "Child",
		Body:    goconfluence.Body{Storage: goconfluence.Storage{Value: ``}},
		Version: &goconfluence.Version{Number: 1},
		Space:   &goconfluence.Space{Key: "OB"},
	}
	api.children["p"] = &goconfluence.Search{Results: []goconfluence.Results{{ID: "c"}}}

	c, dir := crawlEnv(t, api, emptyAttachments{})

	if err := c.Crawl(context.Background(), []string{"p"}); err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "c-child", "page.json")); err != nil {
		t.Errorf("дочерняя страница не выгружена: %v", err)
	}
}

func TestCrawlerContextCancelStops(t *testing.T) {
	api := newStubAPI()
	api.pages["A"] = &goconfluence.Content{
		ID: "A", Title: "Page A",
		Body:    goconfluence.Body{Storage: goconfluence.Storage{Value: `<a href="/pages/viewpage.action?pageId=B">B</a>`}},
		Version: &goconfluence.Version{Number: 1},
		Space:   &goconfluence.Space{Key: "OB"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // отменяем ДО старта

	c, _ := crawlEnv(t, api, emptyAttachments{})
	if err := c.Crawl(ctx, []string{"A"}); err != nil {
		t.Fatalf("Crawl при отменённом контексте должен выйти чисто: %v", err)
	}
	if api.getByIDCalls["A"] != 0 {
		t.Errorf("при отменённом контексте не должно быть запросов, получили %d", api.getByIDCalls["A"])
	}
}
