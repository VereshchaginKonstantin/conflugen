package main

import (
	"encoding/xml"
	"net/url"
	"strings"
)

// parsePageURL превращает href из storage format в pageRef. Пустой pageRef
// (IsZero) означает «это не ссылка на страницу нашего Confluence» — внешний
// хост, вложение, mailto, якорь или мусор. Относительные ссылки считаются
// нашими: host у них пуст.
//
// selfHost — хост нашего Confluence (с портом, если он есть в URL); ссылки на
// другие хосты отбрасываются, чтобы краулер не ушёл гулять по интернету.
func parsePageURL(href, selfHost string) pageRef {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "mailto:") {
		return pageRef{}
	}

	u, err := url.Parse(href)
	if err != nil {
		return pageRef{}
	}
	if u.Host != "" && u.Host != selfHost {
		return pageRef{}
	}

	path := u.Path
	if strings.Contains(path, "/download/attachments/") || strings.Contains(path, "/download/thumbnails/") {
		return pageRef{}
	}

	// 1. …/viewpage.action?pageId=123
	if id := u.Query().Get("pageId"); id != "" {
		return pageRef{ID: id}
	}

	segs := splitPath(path)

	// 2. Cloud: …/spaces/SPACE/pages/123/Title
	for i := 0; i+1 < len(segs); i++ {
		if segs[i] == "pages" && isDigits(segs[i+1]) {
			return pageRef{ID: segs[i+1]}
		}
	}

	// 3. Server/DC: …/display/SPACE/Page+Title
	for i := 0; i+2 < len(segs); i++ {
		if segs[i] != "display" {
			continue
		}
		space := segs[i+1]
		title := decodeTitle(segs[i+2])
		if space != "" && title != "" {
			return pageRef{Title: title, Space: space}
		}
	}

	return pageRef{}
}

// splitPath режет путь на непустые сегменты.
func splitPath(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// decodeTitle раскодирует сегмент пути: `+` в Confluence-ссылках значит пробел,
// а не литеральный плюс, поэтому сначала меняем его, потом раскодируем %XX.
func decodeTitle(seg string) string {
	seg = strings.ReplaceAll(seg, "+", " ")
	if dec, err := url.PathUnescape(seg); err == nil {
		return dec
	}
	return seg
}

// extractLinks вытаскивает из storage format все ссылки на другие страницы
// Confluence. Разбор потоковый (xml.Decoder), а не регулярками: storage format —
// это XML, и `ri:content-title` законно содержит `>`, кавычки и &-энтити.
//
// curSpace — пространство страницы, из которой извлекаем: `<ri:page>` без
// `ri:space-key` ссылается внутрь того же пространства.
//
// Результат дедуплицирован по pageRef.Key() с сохранением порядка появления.
// Битый XML не считается ошибкой: возвращаем всё, что успели разобрать до
// точки поломки, — дамп важнее строгости.
func extractLinks(storage, curSpace, selfHost string) []pageRef {
	dec := xml.NewDecoder(strings.NewReader(storage))
	// Storage format — это XML только на словах: в нём живут HTML-сущности
	// (&nbsp; и родня), которых в XML не существует, и незакрытые теги вроде
	// <br>. В строгом режиме Decoder на первой же такой сущности возвращает
	// ошибку и мы теряем все ссылки после неё. Нестрогий режим пропускает их.
	//
	// Префиксы ac: и ri: приходят без объявлений xmlns — Decoder не считает это
	// ошибкой и просто кладёт сырой префикс в xml.Name.Space. Поэтому ниже мы
	// сравниваем только Local ("page", "a"), игнорируя Space.
	dec.Strict = false

	var (
		out  []pageRef
		seen = map[string]bool{}
	)

	add := func(r pageRef) {
		if r.IsZero() || seen[r.Key()] {
			return
		}
		seen[r.Key()] = true
		out = append(out, r)
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			// io.EOF — нормальный конец; любая другая ошибка — битый XML,
			// возвращаем разобранное.
			return out
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		switch start.Name.Local {
		case "page": // <ri:page ri:content-title="…" ri:space-key="…"/>
			var title, space string
			for _, attr := range start.Attr {
				switch attr.Name.Local {
				case "content-title":
					title = attr.Value
				case "space-key":
					space = attr.Value
				}
			}
			if space == "" {
				space = curSpace
			}
			add(pageRef{Title: title, Space: space})

		case "a": // <a href="…">
			for _, attr := range start.Attr {
				if attr.Name.Local == "href" {
					add(parsePageURL(attr.Value, selfHost))
				}
			}
		}
	}
}
