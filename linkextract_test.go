package main

import "testing"

func TestParsePageURL(t *testing.T) {
	const host = "conf.example.com"

	tests := []struct {
		name string
		href string
		want pageRef
	}{
		{
			name: "viewpage.action с pageId",
			href: "https://conf.example.com/pages/viewpage.action?pageId=123",
			want: pageRef{ID: "123"},
		},
		{
			name: "относительный viewpage.action",
			href: "/pages/viewpage.action?pageId=456",
			want: pageRef{ID: "456"},
		},
		{
			name: "cloud-путь /spaces/OB/pages/789/Title",
			href: "https://conf.example.com/spaces/OB/pages/789/Some+Title",
			want: pageRef{ID: "789"},
		},
		{
			name: "display с плюсами и процентами",
			href: "https://conf.example.com/display/OB/On-call+%D0%B4%D0%B5%D0%B6%D1%83%D1%80%D1%81%D1%82%D0%B2%D0%BE",
			want: pageRef{Title: "On-call дежурство", Space: "OB"},
		},
		{
			name: "чужой хост игнорируется",
			href: "https://example.org/pages/viewpage.action?pageId=1",
			want: pageRef{},
		},
		{
			name: "вложение игнорируется",
			href: "https://conf.example.com/download/attachments/123/pic.png",
			want: pageRef{},
		},
		{
			name: "mailto игнорируется",
			href: "mailto:a@b.c",
			want: pageRef{},
		},
		{
			name: "голый якорь игнорируется",
			href: "#section",
			want: pageRef{},
		},
		{
			name: "мусор не роняет парсер",
			href: "://[",
			want: pageRef{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePageURL(tt.href, host)
			if got != tt.want {
				t.Errorf("parsePageURL(%q) = %+v, хотим %+v", tt.href, got, tt.want)
			}
		})
	}
}

func TestExtractLinks(t *testing.T) {
	const host = "conf.example.com"

	tests := []struct {
		name     string
		storage  string
		curSpace string
		want     []pageRef
	}{
		{
			name:     "ri:page с явным space",
			storage:  `<p><ac:link><ri:page ri:content-title="On-call" ri:space-key="OB"/></ac:link></p>`,
			curSpace: "DEV",
			want:     []pageRef{{Title: "On-call", Space: "OB"}},
		},
		{
			name:     "ri:page без space наследует space страницы",
			storage:  `<ac:link><ri:page ri:content-title="Runbooks"/></ac:link>`,
			curSpace: "DEV",
			want:     []pageRef{{Title: "Runbooks", Space: "DEV"}},
		},
		{
			name:     "заголовок с угловой скобкой и амперсандом",
			storage:  `<ac:link><ri:page ri:content-title="A &gt; B &amp; C" ri:space-key="OB"/></ac:link>`,
			curSpace: "DEV",
			want:     []pageRef{{Title: "A > B & C", Space: "OB"}},
		},
		{
			name:     "обычная ссылка a href",
			storage:  `<p>см. <a href="https://conf.example.com/pages/viewpage.action?pageId=42">тут</a></p>`,
			curSpace: "DEV",
			want:     []pageRef{{ID: "42"}},
		},
		{
			name:     "внешняя ссылка и якорь отбрасываются",
			storage:  `<a href="https://example.org/x">x</a><a href="#top">top</a>`,
			curSpace: "DEV",
			want:     nil,
		},
		{
			name:     "ссылка на вложение отбрасывается",
			storage:  `<a href="/download/attachments/1/a.png">pic</a>`,
			curSpace: "DEV",
			want:     nil,
		},
		{
			name:     "дубликаты схлопываются, порядок сохраняется",
			storage:  `<a href="/pages/viewpage.action?pageId=7">a</a><a href="/pages/viewpage.action?pageId=7">b</a><a href="/pages/viewpage.action?pageId=8">c</a>`,
			curSpace: "DEV",
			want:     []pageRef{{ID: "7"}, {ID: "8"}},
		},
		{
			name:     "фрагмент без единого корня",
			storage:  `<p>раз</p><p><a href="/pages/viewpage.action?pageId=9">два</a></p>`,
			curSpace: "DEV",
			want:     []pageRef{{ID: "9"}},
		},
		{
			name:     "битый XML не роняет — возвращаем что успели",
			storage:  `<a href="/pages/viewpage.action?pageId=5">ок</a><p unclosed=`,
			curSpace: "DEV",
			want:     []pageRef{{ID: "5"}},
		},
		{
			name:     "пустое тело",
			storage:  ``,
			curSpace: "DEV",
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractLinks(tt.storage, tt.curSpace, host)
			if len(got) != len(tt.want) {
				t.Fatalf("extractLinks вернул %d ссылок (%+v), хотим %d (%+v)", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ссылка %d = %+v, хотим %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
