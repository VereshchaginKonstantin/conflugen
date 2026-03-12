package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Directive содержит параметры синхронизации из md файла
type Directive struct {
	ParentID string
	SpaceKey string
	Title    string
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

	if directive.ParentID == "" {
		return nil, nil, fmt.Errorf("directive missing required param: parent-id")
	}

	if directive.SpaceKey == "" {
		return nil, nil, fmt.Errorf("directive missing required param: space-key")
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
		case "parent-id":
			d.ParentID = value
		case "space-key":
			d.SpaceKey = value
		case "title":
			d.Title = value
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
