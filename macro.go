package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Macro — пара regex → шаблон. Применяется к raw markdown ДО goldmark,
// поэтому в шаблоне можно писать готовый storage-XML (`<ac:…>` пройдёт через
// существующий unescapeConfluenceMacros).
//
// Template поддерживает Go regex Expand-синтаксис: `$0` — полное совпадение,
// `$1`/`$2`/… — capture-группы; `${name}` — именованные группы.
type Macro struct {
	Pattern  *regexp.Regexp
	Template string
}

// macroDirectiveRe ищет: <!-- +conflugen-macro PATTERN => TEMPLATE -->
// (?s) — точка матчит \n, чтобы шаблоны умели быть мультистрочными.
var macroDirectiveRe = regexp.MustCompile(`(?s)<!--\s*\+conflugen-macro\s+(.+?)\s+=>\s+(.+?)\s*-->`)

// useDirectiveRe ищет: <!-- +conflugen-use NAME [k=v ...] -->
var useDirectiveRe = regexp.MustCompile(`<!--\s*\+conflugen-use\s+([^>]+?)\s*-->`)

// ExtractMacros находит и удаляет +conflugen-macro директивы; возвращает
// список Macro в порядке появления и контент без этих директив.
func ExtractMacros(content []byte) ([]Macro, []byte, error) {
	matches := macroDirectiveRe.FindAllSubmatchIndex(content, -1)
	var (
		macros []Macro
		out    strings.Builder
		pos    int
	)
	for _, m := range matches {
		out.Write(content[pos:m[0]])
		pos = m[1]
		rawPattern := strings.TrimSpace(string(content[m[2]:m[3]]))
		rawTemplate := strings.TrimSpace(string(content[m[4]:m[5]]))
		re, err := regexp.Compile(rawPattern)
		if err != nil {
			return nil, nil, fmt.Errorf("macro pattern %q: %w", rawPattern, err)
		}
		macros = append(macros, Macro{Pattern: re, Template: rawTemplate})
	}
	out.Write(content[pos:])
	return macros, []byte(out.String()), nil
}

// EnableStdlibPacks находит и удаляет +conflugen-use директивы; раскрывает их
// в preset-макросы из встроенной библиотеки. existing — макросы, уже собранные
// ExtractMacros; на выходе они дополнены preset'ами.
func EnableStdlibPacks(content []byte, existing []Macro) ([]Macro, []byte, error) {
	matches := useDirectiveRe.FindAllSubmatchIndex(content, -1)
	var (
		out strings.Builder
		pos int
	)
	macros := existing
	for _, m := range matches {
		out.Write(content[pos:m[0]])
		pos = m[1]
		rawArgs := strings.TrimSpace(string(content[m[2]:m[3]]))
		pack, err := resolveStdlibPack(rawArgs)
		if err != nil {
			return nil, nil, err
		}
		macros = append(macros, pack...)
	}
	out.Write(content[pos:])
	return macros, []byte(out.String()), nil
}

// ApplyMacros применяет каждую Macro к content в порядке регистрации.
// regex Expand-синтаксис обрабатывается стандартным ReplaceAll.
func ApplyMacros(content []byte, macros []Macro) []byte {
	for _, m := range macros {
		content = m.Pattern.ReplaceAll(content, []byte(m.Template))
	}
	return content
}
