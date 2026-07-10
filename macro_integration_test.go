package main

import (
	"strings"
	"testing"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

// TestMacroPipelineEndToEnd: макрос вставляет raw `<ac:…>` в markdown,
// goldmark экранирует его, unescapeConfluenceMacros восстанавливает —
// итоговый HTML содержит storage-XML целиком.
func TestMacroPipelineEndToEnd(t *testing.T) {
	t.Parallel()

	src := []byte("<!-- +conflugen-use jira project=PLAT -->\n\n# Doc\n\nfix PLAT-7 ASAP\n")

	macros, body, err := ExtractMacros(src)
	assertNoError(t, err)
	macros, body, err = EnableStdlibPacks(body, macros)
	assertNoError(t, err)
	body = ApplyMacros(body, macros)

	md := newMarkdownConverter(extensions.NewMermaidCollector(), extensions.NewLayoutCollector(), extensions.NewImageCollector())
	htmlContent, _, err := convertMarkdown(md, body)
	assertNoError(t, err)

	if !strings.Contains(htmlContent, `<ac:structured-macro ac:name="jira"`) {
		t.Fatalf("jira-макрос не дошёл в финальный HTML:\n%s", htmlContent)
	}
	if !strings.Contains(htmlContent, `<ac:parameter ac:name="key">PLAT-7</ac:parameter>`) {
		t.Fatalf("capture не подставлен:\n%s", htmlContent)
	}
	if strings.Contains(htmlContent, "&lt;ac:") {
		t.Fatalf("остался экранированный ac:, нужен passthrough:\n%s", htmlContent)
	}
}

// TestMacroAndBoxPipelineEndToEnd проверяет, что box-макрос даёт корректный
// storage-XML, и Confluence-обёртка вокруг параграфа корректно снимается
// unescapeConfluenceMacros (паттерн «вытащить макрос из <p>…</p>»).
// ВАЖНО: <ac:rich-text-body> без атрибутов CommonMark-парсер посчитал бы
// autolink'ом — поэтому в шаблоне на нём ac:schema-version="1".
// Регрессия закрывает баг, при котором Confluence отвергал страницу с
// «Unexpected close tag </code>; expected </ac:layout-section>».
func TestMacroAndBoxPipelineEndToEnd(t *testing.T) {
	t.Parallel()

	src := []byte("<!-- +conflugen-use box -->\n[info: ключевая мысль]\n")
	macros, body, _ := ExtractMacros(src)
	macros, body, _ = EnableStdlibPacks(body, macros)
	body = ApplyMacros(body, macros)

	md := newMarkdownConverter(extensions.NewMermaidCollector(), extensions.NewLayoutCollector(), extensions.NewImageCollector())
	htmlContent, _, err := convertMarkdown(md, body)
	assertNoError(t, err)

	if !strings.Contains(htmlContent, `<ac:structured-macro ac:name="info"`) {
		t.Fatalf("нет info-макроса:\n%s", htmlContent)
	}
	if !strings.Contains(htmlContent, "<ac:rich-text-body") {
		t.Fatalf("нет ac:rich-text-body (autolink-баг вернулся?):\n%s", htmlContent)
	}
	if strings.Contains(htmlContent, `<a href="ac:rich-text-body">`) {
		t.Fatalf("autolink не должен срабатывать на <ac:rich-text-body>:\n%s", htmlContent)
	}
	if !strings.Contains(htmlContent, "ключевая мысль") {
		t.Fatalf("текст потерян:\n%s", htmlContent)
	}
}
