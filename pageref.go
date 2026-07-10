package main

import "fmt"

// pageRef — ссылка на страницу Confluence, ещё не обязательно резолвленная в id.
// Из `<ac:link><ri:page ri:content-title=…>` мы узнаём только заголовок и
// пространство; из `viewpage.action?pageId=123` — сразу id. Краулер приводит
// всё к id, но дедуплицировать очередь надо и до резолва — для этого Key().
type pageRef struct {
	ID    string
	Title string
	Space string
}

// Key — стабильный ключ для дедупликации. Ref с id и ref с title+space,
// указывающие на одну страницу, дадут разные ключи: это осознанно. Резолв
// title→id всё равно произойдёт, и уже id-ключ отсеет повтор в visited.
func (r pageRef) Key() string {
	if r.ID != "" {
		return "id:" + r.ID
	}
	return fmt.Sprintf("title:%s/%s", r.Space, r.Title)
}

// IsZero — ref неадресуем: нет ни id, ни пары title+space.
// Заголовок без пространства искать негде, поэтому это тоже «пусто».
func (r pageRef) IsZero() bool {
	return r.ID == "" && (r.Title == "" || r.Space == "")
}
