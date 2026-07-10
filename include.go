package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// includeRegex находит строку-директиву `<!-- +conflugen-include <path> -->`.
// path может быть относительным (от каталога включающего файла) или абсолютным.
var includeRegex = regexp.MustCompile(`<!--\s*\+conflugen-include\s+(\S+?)\s*-->`)

// ExpandIncludes раскрывает все `<!-- +conflugen-include <path> -->` в content,
// рекурсивно заменяя их содержимым указанных файлов. Применяется ДО goldmark,
// поэтому раскрытое содержимое участвует во всех остальных фичах
// (parent-by-title, ::: columns, mermaid и т.д.) без специальной обработки.
//
// baseDir — каталог, относительно которого резолвятся пути include в этом content.
// Для верхнего вызова это `filepath.Dir(<входной .md>)`.
//
// seen — абсолютные пути уже включённых файлов в текущей цепочке (защита от циклов).
// На каждом шаге передаётся копия + текущий файл — параллельные ветки могут
// включать один и тот же файл, обратное включение ловится.
func ExpandIncludes(baseDir string, content []byte, seen map[string]bool) ([]byte, error) {
	if seen == nil {
		seen = map[string]bool{}
	}

	var out strings.Builder
	pos := 0
	for _, m := range includeRegex.FindAllSubmatchIndex(content, -1) {
		start, end := m[0], m[1]
		pathStart, pathEnd := m[2], m[3]
		out.Write(content[pos:start])
		pos = end

		raw := string(content[pathStart:pathEnd])
		includePath := raw
		if !filepath.IsAbs(includePath) {
			includePath = filepath.Join(baseDir, includePath)
		}
		includePath = filepath.Clean(includePath)

		if seen[includePath] {
			return nil, fmt.Errorf("include цикл: %s уже включён выше по цепочке", includePath)
		}

		data, err := os.ReadFile(includePath)
		if err != nil {
			return nil, fmt.Errorf("read include %q: %w", raw, err)
		}

		childSeen := make(map[string]bool, len(seen)+1)
		for k, v := range seen {
			childSeen[k] = v
		}
		childSeen[includePath] = true

		expanded, err := ExpandIncludes(filepath.Dir(includePath), data, childSeen)
		if err != nil {
			return nil, err
		}
		out.Write(expanded)
	}
	out.Write(content[pos:])
	return []byte(out.String()), nil
}
