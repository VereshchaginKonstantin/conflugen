package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// translit — таблица транслитерации кириллицы. Без неё slug русского заголовка
// схлопнулся бы в сплошные дефисы и папки стали бы неразличимы.
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "c", 'ч': "ch", 'ш': "sh", 'щ': "sch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

const maxSlugLen = 60

// errDiskWrite оборачивает всё, что сломалось при записи на диск. Такая ошибка
// фатальна: продолжать обход, когда дамп некуда писать, бессмысленно. Краулер
// отличает её от сетевой ошибки на конкретном вложении через errors.Is.
var errDiskWrite = errors.New("ошибка записи на диск")

// slugify превращает заголовок страницы в безопасное имя папки: транслитерация
// кириллицы, нижний регистр, всё прочее — в дефис, повторы сжимаются, обрезка
// до maxSlugLen. Пустой результат допустим: вызывающий добавит pageId.
func slugify(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		// Проверяем наличие в таблице, а не непустоту значения: `ъ` и `ь`
		// сопоставлены пустой строке намеренно — они должны исчезать, а не
		// превращаться в дефис.
		if s, ok := translit[r]; ok {
			b.WriteString(s)
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('-')
	}

	s := collapseDashes(b.String())
	// Транслитерация оставляет только ASCII, поэтому резать по байтам безопасно.
	if len(s) > maxSlugLen {
		s = strings.Trim(s[:maxSlugLen], "-")
	}
	return s
}

// collapseDashes сжимает серии дефисов в один и срезает крайние.
func collapseDashes(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if r == '-' {
			if !prevDash {
				b.WriteRune('-')
			}
			prevDash = true
			continue
		}
		b.WriteRune(r)
		prevDash = false
	}
	return strings.Trim(b.String(), "-")
}

// pageDirName — имя папки страницы. pageId впереди: он уникален при совпадении
// заголовков и переживает переименование страницы, так что resume найдёт папку.
func pageDirName(pageID, title string) string {
	slug := slugify(title)
	if slug == "" {
		return pageID
	}
	return pageID + "-" + slug
}

// sanitizeFilename делает имя вложения безопасным. Транслитерации здесь нет:
// читаемость имени и его расширение важнее ASCII-чистоты.
//
// Порядок шагов важен. Сначала режем на сегменты по обоим разделителям пути и
// выбрасываем `.` и `..` — иначе вложение с именем `../../etc/passwd` (а имя
// вложения приходит с сервера и доверять ему нельзя) записалось бы за пределы
// папки дампа. Только потом чистим оставшиеся символы.
func sanitizeFilename(name string) string {
	var segments []string
	for _, seg := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == "." || seg == ".." {
			continue
		}
		segments = append(segments, seg)
	}

	var b strings.Builder
	for i, seg := range segments {
		if i > 0 {
			b.WriteRune('-')
		}
		for _, r := range seg {
			if r == ':' || unicode.IsControl(r) {
				b.WriteRune('-')
				continue
			}
			b.WriteRune(r)
		}
	}

	out := collapseDashes(b.String())
	if out == "" || out == "." || out == ".." {
		return "attachment"
	}
	return out
}

// writeFileAtomic пишет файл так, чтобы прерывание (Ctrl+C, паника, kill) не
// оставило обрезанного содержимого: сначала во временный файл в той же
// директории, потом Rename на место. Rename в пределах одной ФС атомарен, и
// читатель видит либо старую версию, либо новую целиком — никогда половину.
//
// Временный файл создаётся рядом с целевым (а не в os.TempDir), потому что
// Rename через границу файловых систем не атомарен и вообще может не сработать.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%w: создать директорию %s: %v", errDiskWrite, dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("%w: создать временный файл в %s: %v", errDiskWrite, dir, err)
	}
	tmpName := tmp.Name()

	// При любой ошибке ниже временный файл не должен пережить функцию. После
	// успешного Rename файла с таким именем уже нет, и Remove вернёт ошибку —
	// глотаем её намеренно, чтобы не заводить флаг «успешно ли переименовали».
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("%w: запись %s: %v", errDiskWrite, tmpName, err)
	}
	// Sync до Rename: иначе при падении питания получим пустой файл с новым именем.
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("%w: sync %s: %v", errDiskWrite, tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%w: закрыть %s: %v", errDiskWrite, tmpName, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("%w: переименовать %s → %s: %v", errDiskWrite, tmpName, path, err)
	}
	return nil
}

// pageMeta — содержимое page.json. Пишется последним из всех файлов страницы:
// его наличие и есть маркер «страница выгружена целиком».
//
// AncestorIDs — только идентификаторы, без заголовков: goconfluence.Ancestor
// содержит единственное поле ID.
type pageMeta struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	SpaceKey     string   `json:"spaceKey"`
	Version      int      `json:"version"`
	AncestorIDs  []string `json:"ancestorIDs"`
	Labels       []string `json:"labels"`
	WebURL       string   `json:"webURL"`
	DownloadedAt string   `json:"downloadedAt"`
	Attachments  []string `json:"attachments"`
}

// indexPage — запись о странице в index.json. Error непуст, если страницу не
// удалось получить: дамп честно фиксирует дыры, а не делает вид, что их нет.
type indexPage struct {
	ID      string `json:"id"`
	Title   string `json:"title,omitempty"`
	Dir     string `json:"dir,omitempty"`
	Version int    `json:"version,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

// indexEdge — ребро краула. Kind: "root" (страница из --list), "child"
// (дочерняя страница) либо "link" (ссылка из тела). From пуст для "root".
type indexEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// dumpIndex — содержимое index.json: карта дампа целиком.
type dumpIndex struct {
	Pages []indexPage `json:"pages"`
	Edges []indexEdge `json:"edges"`
}

// pageStore отвечает за раскладку дампа на диске и ни за что больше: он не
// знает ни про Confluence, ни про HTTP. Благодаря этому тестируется в t.TempDir()
// без единого мока.
type pageStore struct {
	root string
	// usedNames: pageID -> множество уже занятых имён вложений. Нужен, чтобы
	// два разных исходных имени, схлопнувшихся санитизацией в одно, не затёрли
	// друг друга.
	usedNames map[string]map[string]bool
	index     dumpIndex
}

func newPageStore(root string) *pageStore {
	return &pageStore{
		root:      root,
		usedNames: make(map[string]map[string]bool),
	}
}

// pageDir — абсолютный путь к папке страницы.
func (s *pageStore) pageDir(pageID, title string) string {
	return filepath.Join(s.root, pageDirName(pageID, title))
}

// SaveXHTML пишет тело страницы байт в байт, без обработки.
func (s *pageStore) SaveXHTML(pageID, title, storage string) error {
	return writeFileAtomic(filepath.Join(s.pageDir(pageID, title), "page.xhtml"), []byte(storage))
}

// SaveAttachment пишет вложение под уже вычисленным (через AttachmentName) именем.
func (s *pageStore) SaveAttachment(pageID, title, filename string, data []byte) error {
	return writeFileAtomic(filepath.Join(s.pageDir(pageID, title), "attachments", filename), data)
}

// SaveMeta пишет page.json. Вызывать ТОЛЬКО после того, как записаны page.xhtml
// и все вложения: этот файл — маркер полноты, и появиться он должен последним.
func (s *pageStore) SaveMeta(m pageMeta) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("сериализовать page.json для %s: %w", m.ID, err)
	}
	return writeFileAtomic(filepath.Join(s.pageDir(m.ID, m.Title), "page.json"), data)
}

// IsComplete отвечает, выгружена ли страница целиком в текущей версии.
// Читает page.json; его отсутствие означает «нет» даже если page.xhtml на месте
// (так выглядит страница, у которой упало вложение).
func (s *pageStore) IsComplete(pageID, title string, version int) bool {
	raw, err := os.ReadFile(filepath.Join(s.pageDir(pageID, title), "page.json"))
	if err != nil {
		return false
	}
	var m pageMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	return m.Version == version
}

// AttachmentName даёт безопасное и уникальное в пределах страницы имя файла.
// Коллизия (два исходных имени схлопнулись санитизацией в одно) разрешается
// суффиксом -2, -3, … перед расширением.
func (s *pageStore) AttachmentName(pageID, rawTitle string) string {
	if s.usedNames[pageID] == nil {
		s.usedNames[pageID] = make(map[string]bool)
	}
	used := s.usedNames[pageID]

	base := sanitizeFilename(rawTitle)
	if !used[base] {
		used[base] = true
		return base
	}

	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 2; ; i++ {
		candidate := stem + "-" + strconv.Itoa(i) + ext
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

// RecordPage добавляет страницу в индекс. Повторный вызов для того же id
// перезаписывает запись: страница могла сначала упасть, а при resume — скачаться.
func (s *pageStore) RecordPage(p indexPage) {
	for i := range s.index.Pages {
		if s.index.Pages[i].ID == p.ID {
			s.index.Pages[i] = p
			return
		}
	}
	s.index.Pages = append(s.index.Pages, p)
}

// RecordEdge добавляет ребро краула, если такого ещё нет.
func (s *pageStore) RecordEdge(from, to, kind string) {
	e := indexEdge{From: from, To: to, Kind: kind}
	for _, existing := range s.index.Edges {
		if existing == e {
			return
		}
	}
	s.index.Edges = append(s.index.Edges, e)
}

// FlushIndex сбрасывает индекс на диск. Вызывается после КАЖДОЙ страницы, а не
// в конце обхода: обход не ограничен и штатно завершается по Ctrl+C, так что
// «конца» может не наступить.
func (s *pageStore) FlushIndex() error {
	data, err := json.MarshalIndent(s.index, "", "  ")
	if err != nil {
		return fmt.Errorf("сериализовать index.json: %w", err)
	}
	return writeFileAtomic(filepath.Join(s.root, "index.json"), data)
}
