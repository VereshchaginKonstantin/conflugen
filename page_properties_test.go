package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// stubRaw — простой rawRequester для тестов: проксирует к httptest.Server.
type stubRaw struct{ client *http.Client }

func (r stubRaw) Request(req *http.Request) ([]byte, error) {
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func TestSetContentAppearanceCreatesWhenMissing(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	posts := map[string]map[string]any{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/property/"):
			http.Error(w, "Not Found", http.StatusNotFound)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/property"):
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			posts[b["key"].(string)] = b
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	raw := stubRaw{client: srv.Client()}
	if err := setContentAppearance(raw, srv.URL, "42", "full-width"); err != nil {
		t.Fatalf("setContentAppearance: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, key := range []string{"content-appearance-draft", "content-appearance-published"} {
		got, ok := posts[key]
		if !ok {
			t.Fatalf("не было POST для %s; есть только: %v", key, mapKeys(posts))
		}
		if got["value"] != "full-width" {
			t.Fatalf("%s: ожидался value=full-width, got %v", key, got["value"])
		}
	}
}

func TestSetContentAppearanceUpdatesWhenPresent(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	puts := map[string]map[string]any{}
	current := map[string]string{
		"content-appearance-draft":     "fixed-width",
		"content-appearance-published": "fixed-width",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			parts := strings.Split(r.URL.Path, "/property/")
			key := parts[len(parts)-1]
			val, ok := current[key]
			if !ok {
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}
			_, _ = fmt.Fprintf(w, `{"key":%q,"value":%q,"version":{"number":7}}`, key, val)
		case http.MethodPut:
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			parts := strings.Split(r.URL.Path, "/property/")
			key := parts[len(parts)-1]
			puts[key] = b
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	raw := stubRaw{client: srv.Client()}
	if err := setContentAppearance(raw, srv.URL, "42", "full-width"); err != nil {
		t.Fatalf("setContentAppearance: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, key := range []string{"content-appearance-draft", "content-appearance-published"} {
		got, ok := puts[key]
		if !ok {
			t.Fatalf("не было PUT для %s", key)
		}
		if got["value"] != "full-width" {
			t.Fatalf("%s: ожидался value=full-width, got %v", key, got["value"])
		}
		ver, _ := got["version"].(map[string]any)
		if ver == nil || ver["number"].(float64) != 8 {
			t.Fatalf("%s: ожидалась version.number=8, got %v", key, got["version"])
		}
	}
}

func TestSetContentAppearanceNoOpWhenAlready(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		putCount int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			parts := strings.Split(r.URL.Path, "/property/")
			key := parts[len(parts)-1]
			_, _ = fmt.Fprintf(w, `{"key":%q,"value":"full-width","version":{"number":1}}`, key)
		case http.MethodPut, http.MethodPost:
			putCount++
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)

	raw := stubRaw{client: srv.Client()}
	if err := setContentAppearance(raw, srv.URL, "42", "full-width"); err != nil {
		t.Fatalf("setContentAppearance: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if putCount != 0 {
		t.Fatalf("ожидался no-op, было %d записывающих запросов", putCount)
	}
}

func mapKeys(m map[string]map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
