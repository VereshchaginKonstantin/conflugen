package main

import (
	"strings"
	"testing"
)

func TestUnwrapParagraphsAroundMacros(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in, out string
	}{
		{
			"простой <p> с макросом — без вложенного <p>",
			`<p><ac:structured-macro ac:name="toc"/></p>`,
			`<ac:structured-macro ac:name="toc"/>`,
		},
		{
			"<p> без ac: — оставляем",
			`<p>обычный текст</p>`,
			`<p>обычный текст</p>`,
		},
		{
			"box-макрос с собственным <p> внутри — снимаем только внешний",
			`<p><ac:structured-macro ac:name="info"><ac:rich-text-body><p>тело</p></ac:rich-text-body></ac:structured-macro></p>`,
			`<ac:structured-macro ac:name="info"><ac:rich-text-body><p>тело</p></ac:rich-text-body></ac:structured-macro>`,
		},
		{
			"несколько подряд: один с макросом, второй без",
			`<p><ac:structured-macro ac:name="x"/></p>` + "\n" + `<p>чистый</p>`,
			`<ac:structured-macro ac:name="x"/>` + "\n" + `<p>чистый</p>`,
		},
		{
			"вложенные ac: и текст — снимаем внешний",
			`<p><ac:structured-macro><ac:rich-text-body><p>line1</p><p>line2</p></ac:rich-text-body></ac:structured-macro></p>`,
			`<ac:structured-macro><ac:rich-text-body><p>line1</p><p>line2</p></ac:rich-text-body></ac:structured-macro>`,
		},
		{
			"непарный <p> — не падаем",
			`<p>без закрытия`,
			`<p>без закрытия`,
		},
		{
			"inline ac: посреди текста — НЕ снимаем <p>",
			`<p>fix <ac:structured-macro ac:name="jira"><ac:parameter ac:name="key">FOO-1</ac:parameter></ac:structured-macro> ASAP</p>`,
			`<p>fix <ac:structured-macro ac:name="jira"><ac:parameter ac:name="key">FOO-1</ac:parameter></ac:structured-macro> ASAP</p>`,
		},
		{
			"inline ac:emoticon в середине параграфа — НЕ снимаем",
			`<p>hello <ac:emoticon ac:name="smile"/> world</p>`,
			`<p>hello <ac:emoticon ac:name="smile"/> world</p>`,
		},
		{
			"параграф НАЧИНАЕТСЯ с пробелов и потом <ac:> — снимаем",
			`<p>   <ac:structured-macro ac:name="toc"/></p>`,
			`<ac:structured-macro ac:name="toc"/>`,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := unwrapParagraphsAroundMacros(c.in)
			if got != c.out {
				t.Fatalf("\nin:   %s\nwant: %s\ngot:  %s", c.in, c.out, got)
			}
		})
	}
}

// TestUnescapeFixesBoxInsideLayoutCell — регрессия на «Unexpected close tag
// </ac:layout-cell>; expected </ac:rich-text-body>»: внутри layout-cell
// после ApplyMacros получается параграф вида
//
//	<p><ac:structured-macro ac:name="info">…<p>тело</p>…</ac:structured-macro></p>
//
// Прежний regex-стрип съедал внутренний </p>, и в выводе оставался
// поломанный <ac:rich-text-body…>…<p>тело</ac:rich-text-body></ac:…></p>.
func TestUnescapeFixesBoxInsideLayoutCell(t *testing.T) {
	t.Parallel()
	in := `<ac:layout-cell><p><ac:structured-macro ac:name="info" ac:schema-version="1"><ac:rich-text-body ac:schema-version="1"><p>тело</p></ac:rich-text-body></ac:structured-macro></p></ac:layout-cell>`
	out := unescapeConfluenceMacros(in)
	if !strings.Contains(out, `<ac:rich-text-body ac:schema-version="1"><p>тело</p></ac:rich-text-body>`) {
		t.Fatalf("внутренний <p>тело</p> должен быть цел:\n%s", out)
	}
	if strings.Contains(out, `<p>тело</ac:rich-text-body>`) {
		t.Fatalf("осталась дырка от старого regex-стрипа:\n%s", out)
	}
}
