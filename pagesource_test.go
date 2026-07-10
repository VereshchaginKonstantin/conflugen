package main

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	goconfluence "github.com/virtomize/confluence-go-api"
)

// fakeRaw — стаб rawRequester: отдаёт заранее заготовленный ответ по URL.
type fakeRaw struct {
	responses map[string][]byte
	calls     []string
	err       error
}

func (s *fakeRaw) Request(req *http.Request) ([]byte, error) {
	s.calls = append(s.calls, req.URL.String())
	if s.err != nil {
		return nil, s.err
	}
	if body, ok := s.responses[req.URL.String()]; ok {
		return body, nil
	}
	return nil, fmt.Errorf("нет заготовки для %s", req.URL.String())
}

func TestPageSourceGetPage(t *testing.T) {
	api := newStubAPI()
	api.pages["123"] = &goconfluence.Content{
		ID:        "123",
		Title:     "Арх",
		Body:      goconfluence.Body{Storage: goconfluence.Storage{Value: "<p>тело</p>"}},
		Version:   &goconfluence.Version{Number: 5},
		Space:     &goconfluence.Space{Key: "OB"},
		Ancestors: []goconfluence.Ancestor{{ID: "1"}, {ID: "2"}},
		Links:     &goconfluence.Links{Base: "https://conf.example.com", WebUI: "/display/OB/Arh"},
	}
	api.labels["123"] = []string{"arch", "docs"}

	src := newPageSource(api, &fakeRaw{}, "https://conf.example.com/rest/api")

	page, err := src.GetPage("123")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page.Storage != "<p>тело</p>" {
		t.Errorf("Storage = %q", page.Storage)
	}
	if page.Meta.Version != 5 || page.Meta.SpaceKey != "OB" {
		t.Errorf("Meta = %+v", page.Meta)
	}
	if len(page.Meta.AncestorIDs) != 2 || page.Meta.AncestorIDs[0] != "1" {
		t.Errorf("AncestorIDs = %v", page.Meta.AncestorIDs)
	}
	if len(page.Meta.Labels) != 2 || page.Meta.Labels[0] != "arch" {
		t.Errorf("Labels = %v", page.Meta.Labels)
	}
	if page.Meta.WebURL != "https://conf.example.com/display/OB/Arh" {
		t.Errorf("WebURL = %q", page.Meta.WebURL)
	}
}

func TestPageSourceGetPageAuthErrorIsFatal(t *testing.T) {
	api := newStubAPI()
	api.errByID = map[string]error{"123": fmt.Errorf("authentication failed")}

	src := newPageSource(api, &fakeRaw{}, "https://conf.example.com/rest/api")

	_, err := src.GetPage("123")
	if err == nil {
		t.Fatal("ожидали ошибку")
	}
	if !errors.Is(err, errAuth) {
		t.Errorf("401 должен превращаться в errAuth, получили %v", err)
	}
}

func TestSiteRootFromAPIURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://conf.example.com/rest/api", "https://conf.example.com"},
		{"https://conf.example.com/rest/api/", "https://conf.example.com"},
		{"https://mycompany.atlassian.net/wiki/rest/api", "https://mycompany.atlassian.net/wiki"},
		{"https://conf.example.com", "https://conf.example.com"},
	}
	for _, tt := range tests {
		if got := siteRootFromAPIURL(tt.in); got != tt.want {
			t.Errorf("siteRootFromAPIURL(%q) = %q, хотим %q", tt.in, got, tt.want)
		}
	}
}

func TestPageSourceListAttachments(t *testing.T) {
	raw := &fakeRaw{responses: map[string][]byte{
		"https://conf.example.com/rest/api/content/123/child/attachment?limit=50&start=0": []byte(`{
			"results": [
				{"id":"att1","title":"diagram.svg","_links":{"download":"/download/attachments/123/diagram.svg?version=1"}}
			],
			"size": 1, "limit": 50
		}`),
	}}

	src := newPageSource(newStubAPI(), raw, "https://conf.example.com/rest/api")

	atts, err := src.ListAttachments("123")
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(atts) != 1 || atts[0].Title != "diagram.svg" {
		t.Fatalf("вложения = %+v", atts)
	}
	if atts[0].DownloadPath != "/download/attachments/123/diagram.svg?version=1" {
		t.Errorf("DownloadPath = %q", atts[0].DownloadPath)
	}
}

func TestPageSourceDownloadAttachment(t *testing.T) {
	raw := &fakeRaw{responses: map[string][]byte{
		"https://conf.example.com/download/attachments/123/diagram.svg?version=1": []byte("svg-bytes"),
	}}

	src := newPageSource(newStubAPI(), raw, "https://conf.example.com/rest/api")

	data, err := src.DownloadAttachment("/download/attachments/123/diagram.svg?version=1")
	if err != nil {
		t.Fatalf("DownloadAttachment: %v", err)
	}
	if string(data) != "svg-bytes" {
		t.Errorf("данные = %q", data)
	}
}

func TestPageSourceResolveRefCaches(t *testing.T) {
	api := newStubAPI()
	src := newPageSource(api, &fakeRaw{}, "https://conf.example.com/rest/api")

	id, err := src.ResolveRef(pageRef{ID: "42"})
	if err != nil || id != "42" {
		t.Fatalf("ResolveRef по id = %q, %v", id, err)
	}
	if api.getContentCalls != 0 {
		t.Errorf("ref с id не должен ходить в Confluence, вызовов: %d", api.getContentCalls)
	}

	api.byTitle["OB/On-call"] = "77"

	for i := 0; i < 2; i++ {
		id, err := src.ResolveRef(pageRef{Title: "On-call", Space: "OB"})
		if err != nil || id != "77" {
			t.Fatalf("ResolveRef по title = %q, %v", id, err)
		}
	}
	if api.getContentCalls != 1 {
		t.Errorf("повторный резолв должен браться из кэша, вызовов: %d", api.getContentCalls)
	}
}

func TestPageSourceListChildren(t *testing.T) {
	api := newStubAPI()
	api.children["p"] = &goconfluence.Search{Results: []goconfluence.Results{{ID: "c1"}, {ID: "c2"}}}

	src := newPageSource(api, &fakeRaw{}, "https://conf.example.com/rest/api")

	ids, err := src.ListChildren("p")
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	if len(ids) != 2 || ids[0] != "c1" || ids[1] != "c2" {
		t.Errorf("дети = %v", ids)
	}
}
