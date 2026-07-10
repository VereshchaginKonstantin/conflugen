package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"латиница", "API Reference", "api-reference"},
		{"кириллица транслитерируется", "Архитектура сервиса", "arhitektura-servisa"},
		{"слэши и двоеточия схлопываются", "a/b:c\\d", "a-b-c-d"},
		{"повторные дефисы сжимаются", "a   ---   b", "a-b"},
		{"крайние дефисы срезаются", "  -hello-  ", "hello"},
		{"пустой результат", "!!!", ""},
		{"пустой вход", "", ""},
		{"обрезка до 60 символов", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"ё и ж", "Ёжик", "yozhik"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := slugify(tt.title); got != tt.want {
				t.Errorf("slugify(%q) = %q, хотим %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestPageDirName(t *testing.T) {
	if got := pageDirName("123", "Архитектура сервиса"); got != "123-arhitektura-servisa" {
		t.Errorf("pageDirName = %q", got)
	}
	// Заголовок из одних спецсимволов даёт пустой slug — остаётся голый id
	// без висящего дефиса.
	if got := pageDirName("123", "!!!"); got != "123" {
		t.Errorf("pageDirName с пустым slug = %q, хотим %q", got, "123")
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct{ in, want string }{
		{"diagram.svg", "diagram.svg"},
		{"my file.png", "my file.png"},
		{"../../etc/passwd", "etc-passwd"},
		{"a/b.png", "a-b.png"},
		{"con:trol\x00.txt", "con-trol-.txt"},
		{"", "attachment"},
		{".", "attachment"},
		{"..", "attachment"},
	}
	for _, tt := range tests {
		if got := sanitizeFilename(tt.in); got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, хотим %q", tt.in, got, tt.want)
		}
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "page.xhtml")

	if err := writeFileAtomic(path, []byte("<p>привет</p>")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("читаем обратно: %v", err)
	}
	if string(got) != "<p>привет</p>" {
		t.Errorf("содержимое = %q", got)
	}

	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("остался временный файл %s", e.Name())
		}
	}
}

func TestWriteFileAtomicOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")

	if err := writeFileAtomic(path, []byte("старое")); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("новое")); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "новое" {
		t.Errorf("содержимое = %q, хотим %q", got, "новое")
	}
}

func TestPageStoreSavePageWritesJSONLast(t *testing.T) {
	dir := t.TempDir()
	store := newPageStore(dir)

	meta := pageMeta{
		ID:           "123",
		Title:        "Архитектура сервиса",
		SpaceKey:     "OB",
		Version:      7,
		AncestorIDs:  []string{"1", "2"},
		Labels:       []string{"arch"},
		WebURL:       "https://conf/display/OB/Arch",
		DownloadedAt: "2026-07-10T00:00:00Z",
		Attachments:  []string{"diagram.svg"},
	}

	if err := store.SaveXHTML(meta.ID, meta.Title, "<p>тело</p>"); err != nil {
		t.Fatalf("SaveXHTML: %v", err)
	}
	if err := store.SaveAttachment(meta.ID, meta.Title, "diagram.svg", []byte("svg-bytes")); err != nil {
		t.Fatalf("SaveAttachment: %v", err)
	}
	if err := store.SaveMeta(meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}

	pageDir := filepath.Join(dir, "123-arhitektura-servisa")

	body, err := os.ReadFile(filepath.Join(pageDir, "page.xhtml"))
	if err != nil || string(body) != "<p>тело</p>" {
		t.Errorf("page.xhtml = %q, err=%v", body, err)
	}

	att, err := os.ReadFile(filepath.Join(pageDir, "attachments", "diagram.svg"))
	if err != nil || string(att) != "svg-bytes" {
		t.Errorf("вложение = %q, err=%v", att, err)
	}

	raw, err := os.ReadFile(filepath.Join(pageDir, "page.json"))
	if err != nil {
		t.Fatalf("page.json: %v", err)
	}
	var got pageMeta
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("разбор page.json: %v", err)
	}
	if got.Version != 7 || got.Title != "Архитектура сервиса" || len(got.Attachments) != 1 {
		t.Errorf("page.json = %+v", got)
	}
}

func TestPageStoreIsComplete(t *testing.T) {
	dir := t.TempDir()
	store := newPageStore(dir)

	if store.IsComplete("123", "Арх", 7) {
		t.Error("пустая папка не должна считаться выгруженной")
	}

	if err := store.SaveXHTML("123", "Арх", "<p/>"); err != nil {
		t.Fatal(err)
	}
	if store.IsComplete("123", "Арх", 7) {
		t.Error("без page.json страница не выгружена целиком")
	}

	if err := store.SaveMeta(pageMeta{ID: "123", Title: "Арх", Version: 7}); err != nil {
		t.Fatal(err)
	}
	if !store.IsComplete("123", "Арх", 7) {
		t.Error("page.json с совпадающей версией → выгружена")
	}

	if store.IsComplete("123", "Арх", 8) {
		t.Error("версия изменилась → страница устарела")
	}
}

func TestPageStoreAttachmentNameCollision(t *testing.T) {
	dir := t.TempDir()
	store := newPageStore(dir)

	n1 := store.AttachmentName("123", "a/b.png")
	n2 := store.AttachmentName("123", "a\\b.png")

	if n1 != "a-b.png" {
		t.Errorf("первое имя = %q", n1)
	}
	if n2 != "a-b-2.png" {
		t.Errorf("коллизия должна дать суффикс -2, получили %q", n2)
	}
}

func TestPageStoreIndex(t *testing.T) {
	dir := t.TempDir()
	store := newPageStore(dir)

	store.RecordPage(indexPage{ID: "1", Title: "Root", Dir: "1-root", Version: 3})
	store.RecordPage(indexPage{ID: "2", Title: "", Dir: "", Error: "403 Forbidden"})
	store.RecordEdge("", "1", "root")
	store.RecordEdge("1", "2", "link")

	if err := store.FlushIndex(); err != nil {
		t.Fatalf("FlushIndex: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatalf("index.json: %v", err)
	}

	var idx dumpIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatalf("разбор index.json: %v", err)
	}
	if len(idx.Pages) != 2 {
		t.Fatalf("страниц в индексе %d, хотим 2", len(idx.Pages))
	}
	if idx.Pages[1].Error != "403 Forbidden" {
		t.Errorf("ошибка второй страницы = %q", idx.Pages[1].Error)
	}
	if len(idx.Edges) != 2 || idx.Edges[1].From != "1" || idx.Edges[1].To != "2" || idx.Edges[1].Kind != "link" {
		t.Errorf("рёбра = %+v", idx.Edges)
	}
}

func TestPageStoreFlushIndexIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	store := newPageStore(dir)
	store.RecordPage(indexPage{ID: "1", Dir: "1"})

	if err := store.FlushIndex(); err != nil {
		t.Fatal(err)
	}
	if err := store.FlushIndex(); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(filepath.Join(dir, "index.json"))
	var idx dumpIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	if len(idx.Pages) != 1 {
		t.Errorf("повторный Flush не должен дублировать записи: %d страниц", len(idx.Pages))
	}
}
