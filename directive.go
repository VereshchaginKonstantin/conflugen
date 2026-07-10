package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Directive содержит параметры синхронизации из md файла
type Directive struct {
	// PageID — точечная адресация СУЩЕСТВУЮЩЕЙ страницы по числовому id.
	// Если задан, conflugen обновляет именно её (PUT), минуя поиск по
	// parent-id+title и создание. Нужно, когда родитель не листится
	// (нет прав / индексная страница) или создание под ним запрещено.
	PageID   string
	ParentID string
	Parents  []string // цепочка предков по заголовкам (parent=...), альтернатива parent-id
	SpaceKey string
	Title    string
	Labels   []string // метки страницы (label=..., повторяемый)
	// Type — тип контента: "page" (дефолт) или "blogpost".
	Type string
	// ContentAppearance — внешний вид: "full-width" или "fixed-width".
	// Пустая строка — не трогаем существующее свойство страницы.
	ContentAppearance string
}

var directiveRegex = regexp.MustCompile(`<!--\s*\+conflugen\s+(.+?)\s*-->`)

// ParseDirective парсит директивы conflugen из содержимого md файла и возвращает очищенный контент
func ParseDirective(content []byte) (*Directive, []byte, error) {
	matches := directiveRegex.FindAllSubmatch(content, -1)
	if len(matches) == 0 {
		return nil, content, nil
	}

	directive := &Directive{}

	for _, match := range matches {
		params := string(match[1])
		if err := parseDirectiveParams(directive, params); err != nil {
			return nil, nil, fmt.Errorf("parse directive params: %w", err)
		}
	}

	if directive.SpaceKey == "" {
		return nil, nil, fmt.Errorf("directive missing required param: space-key")
	}

	hasPageID := directive.PageID != ""
	hasParentID := directive.ParentID != ""
	hasParents := len(directive.Parents) > 0
	if hasParentID && hasParents {
		return nil, nil, fmt.Errorf("directive: укажите либо parent-id, либо parent, но не оба сразу")
	}
	// page-id адресует существующую страницу напрямую — родитель не нужен.
	if !hasPageID && !hasParentID && !hasParents {
		return nil, nil, fmt.Errorf("directive missing required param: page-id, parent-id или parent")
	}

	switch directive.Type {
	case "", "page", "blogpost":
		// ok
	default:
		return nil, nil, fmt.Errorf("directive: type должен быть page|blogpost, получено %q", directive.Type)
	}
	switch directive.ContentAppearance {
	case "", "full-width", "fixed-width":
		// ok
	default:
		return nil, nil, fmt.Errorf("directive: content-appearance должен быть full-width|fixed-width, получено %q", directive.ContentAppearance)
	}

	cleaned := directiveRegex.ReplaceAll(content, nil)
	cleaned = trimLeadingEmptyLines(cleaned)

	return directive, cleaned, nil
}

func parseDirectiveParams(d *Directive, params string) error {
	for len(params) > 0 {
		params = strings.TrimSpace(params)
		if params == "" {
			break
		}

		eqIdx := strings.Index(params, "=")
		if eqIdx < 0 {
			return fmt.Errorf("invalid param format: %s", params)
		}

		key := strings.TrimSpace(params[:eqIdx])
		rest := params[eqIdx+1:]

		var value string
		if strings.HasPrefix(rest, `"`) {
			closeIdx := strings.Index(rest[1:], `"`)
			if closeIdx < 0 {
				return fmt.Errorf("unclosed quote for param: %s", key)
			}
			value = rest[1 : closeIdx+1]
			rest = rest[closeIdx+2:]
		} else {
			spaceIdx := strings.IndexByte(rest, ' ')
			if spaceIdx < 0 {
				value = rest
				rest = ""
			} else {
				value = rest[:spaceIdx]
				rest = rest[spaceIdx+1:]
			}
		}

		switch key {
		case "page-id":
			d.PageID = value
		case "parent-id":
			d.ParentID = value
		case "parent":
			d.Parents = append(d.Parents, value)
		case "space-key":
			d.SpaceKey = value
		case "title":
			d.Title = value
		case "label":
			d.Labels = append(d.Labels, value)
		case "type":
			d.Type = value
		case "content-appearance":
			d.ContentAppearance = value
		default:
			return fmt.Errorf("unknown directive param: %s", key)
		}

		params = rest
	}

	return nil
}

func trimLeadingEmptyLines(data []byte) []byte {
	for len(data) > 0 && (data[0] == '\n' || data[0] == '\r') {
		data = data[1:]
	}
	return data
}
