package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPageIDList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pages.txt")

	content := `# страницы для выгрузки
123456789

  123456790
# ещё комментарий
123456791
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readPageIDList([]string{path})
	if err != nil {
		t.Fatalf("readPageIDList: %v", err)
	}

	want := []string{"123456789", "123456790", "123456791"}
	if len(got) != len(want) {
		t.Fatalf("получили %v, хотим %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, хотим %q", i, got[i], want[i])
		}
	}
}

func TestReadPageIDListRejectsNonNumeric(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pages.txt")
	if err := os.WriteFile(path, []byte("https://conf/display/OB/Page\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := readPageIDList([]string{path})
	if err == nil {
		t.Fatal("URL вместо pageId должен давать понятную ошибку, а не молча пропускаться")
	}
}

func TestReadPageIDListMissingFile(t *testing.T) {
	if _, err := readPageIDList([]string{"/nope/nope.txt"}); err == nil {
		t.Fatal("отсутствующий файл списка должен быть ошибкой")
	}
}

func TestHostFromAPIURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://conf.example.com/rest/api", "conf.example.com"},
		{"https://conf.example.com:8443/rest/api", "conf.example.com:8443"},
		{"not a url", ""},
	}
	for _, tt := range tests {
		if got := hostFromAPIURL(tt.in); got != tt.want {
			t.Errorf("hostFromAPIURL(%q) = %q, хотим %q", tt.in, got, tt.want)
		}
	}
}
