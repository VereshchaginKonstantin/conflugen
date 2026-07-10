package main

import "testing"

func TestTransformTaskListCheckboxes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in, out string
	}{
		{
			"checked → ☑",
			`<li><input checked="" disabled="" type="checkbox" /> done</li>`,
			`<li>☑ done</li>`,
		},
		{
			"unchecked → ☐",
			`<li><input disabled="" type="checkbox" /> todo</li>`,
			`<li>☐ todo</li>`,
		},
		{
			"иной порядок атрибутов",
			`<li><input type="checkbox" disabled checked /> done</li>`,
			`<li>☑ done</li>`,
		},
		{
			"без чекбоксов — без изменений",
			`<li>обычный пункт</li>`,
			`<li>обычный пункт</li>`,
		},
		{
			"несколько чекбоксов в одной строке",
			`<li><input disabled="" type="checkbox" /> a</li><li><input checked="" disabled="" type="checkbox" /> b</li>`,
			`<li>☐ a</li><li>☑ b</li>`,
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := transformTaskListCheckboxes(c.in)
			if got != c.out {
				t.Fatalf("\nin:   %s\nwant: %s\ngot:  %s", c.in, c.out, got)
			}
		})
	}
}
