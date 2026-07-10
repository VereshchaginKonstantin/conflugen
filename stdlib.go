package main

import (
	"fmt"
	"regexp"
	"strings"
)

// resolveStdlibPack превращает аргументы +conflugen-use в набор Macro.
// args — строка вида "NAME [k=v ...]" из содержимого комментария.
//
// Доступные пакеты:
//   - toc            — [[toc]] → <ac:structured-macro ac:name="toc"/>
//   - jira [project=KEY] — `KEY-\d+` → JIRA-макрос (дефолт project=JIRA)
//   - status         — [status:Colour Title] → status-бейдж
//   - box            — [info: …] / [tip: …] / [note: …] / [warning: …]
func resolveStdlibPack(args string) ([]Macro, error) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return nil, fmt.Errorf("+conflugen-use: пустое имя пакета")
	}
	name := fields[0]
	params := map[string]string{}
	for _, f := range fields[1:] {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			return nil, fmt.Errorf("+conflugen-use %s: непонятный аргумент %q", name, f)
		}
		params[k] = v
	}

	switch name {
	case "toc":
		return stdlibTOC(), nil
	case "jira":
		return stdlibJira(params), nil
	case "status":
		return stdlibStatus(), nil
	case "box":
		return stdlibBox(), nil
	default:
		return nil, fmt.Errorf("+conflugen-use: неизвестный пакет %q", name)
	}
}

func stdlibTOC() []Macro {
	return []Macro{{
		Pattern:  regexp.MustCompile(`\[\[toc\]\]`),
		Template: `<ac:structured-macro ac:name="toc" ac:schema-version="1"/>`,
	}}
}

// stdlibJira: «PROJ-123» в тексте → JIRA-макрос со ссылкой на тикет.
// Префикс настраивается параметром project=…; дефолт «JIRA».
func stdlibJira(params map[string]string) []Macro {
	project := params["project"]
	if project == "" {
		project = "JIRA"
	}
	pattern := fmt.Sprintf(`\b%s-\d+\b`, regexp.QuoteMeta(project))
	return []Macro{{
		Pattern:  regexp.MustCompile(pattern),
		Template: `<ac:structured-macro ac:name="jira" ac:schema-version="1"><ac:parameter ac:name="key">$0</ac:parameter></ac:structured-macro>`,
	}}
}

// stdlibStatus: [status:Colour Title] → бейдж. Цвет передаём как написано —
// Confluence ожидает капитализированные значения (Green/Red/Yellow/Grey/Blue/Purple).
func stdlibStatus() []Macro {
	return []Macro{{
		Pattern:  regexp.MustCompile(`\[status:([A-Za-z]+)\s+([^\]]+)\]`),
		Template: `<ac:structured-macro ac:name="status" ac:schema-version="1"><ac:parameter ac:name="colour">$1</ac:parameter><ac:parameter ac:name="title">$2</ac:parameter></ac:structured-macro>`,
	}}
}

// stdlibBox: [info|tip|note|warning: одна строка] → callout-макрос с телом-параграфом.
// Для многострочного тела используйте сырой <ac:structured-macro>.
//
// Зачем `ac:schema-version="1"` на `<ac:rich-text-body>`: без атрибутов
// этот тег матчится CommonMark-правилом autolink (`<scheme:rest>` —
// scheme=ac, rest=rich-text-body), и goldmark оборачивает его в <a href=…>.
// Любой атрибут (с пробелом перед `>`) ломает autolink и пускает тег
// дальше как raw inline-HTML. Confluence игнорирует незнакомые атрибуты
// на rich-text-body.
func stdlibBox() []Macro {
	return []Macro{{
		Pattern:  regexp.MustCompile(`\[(info|tip|note|warning):\s+([^\]]+)\]`),
		Template: `<ac:structured-macro ac:name="$1" ac:schema-version="1"><ac:rich-text-body ac:schema-version="1"><p>$2</p></ac:rich-text-body></ac:structured-macro>`,
	}}
}
