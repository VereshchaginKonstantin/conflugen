package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// transformTaskListsToConfluence сканирует html, ищет <ul>…</ul>, у которых
// ВСЕ прямые <li>-дети начинаются с <input type="checkbox"…/>, и заменяет
// такой блок на нативный Confluence-макрос:
//
//	<ac:task-list>
//	  <ac:task>
//	    <ac:task-id>N</ac:task-id>
//	    <ac:task-uuid>…</ac:task-uuid>
//	    <ac:task-status>complete|incomplete</ac:task-status>
//	    <ac:task-body>…</ac:task-body>
//	  </ac:task>
//	  …
//	</ac:task-list>
//
// Это даёт кликабельные таски с persistent-состоянием в Confluence (вместо
// плоских <li>☑/☐, у которых нет интерактива).
//
// UUID детерминированный — sha256 от тела таска, нарезанный в UUID-формат.
// Тогда при повторной публикации того же source markdown id не меняются и
// Confluence не «теряет» галочки, проставленные пользователем в UI.
// Сменился текст таска → сменился UUID → это уже другая задача.
//
// Списки, где не каждый <li> — task-item, не трогаем (Unicode-fallback в
// transformTaskListCheckboxes отработает на оставшихся <input>'ах).
func transformTaskListsToConfluence(html string) string {
	var out strings.Builder
	pos := 0
	taskID := 0
	for pos < len(html) {
		idx := indexTagOpen(html, pos, "ul")
		if idx < 0 {
			out.WriteString(html[pos:])
			return out.String()
		}
		out.WriteString(html[pos:idx])

		// Найти конец открывающего тега и матчинговый </ul>.
		openEnd := strings.Index(html[idx:], ">")
		if openEnd < 0 {
			out.WriteString(html[idx:])
			return out.String()
		}
		openEnd += idx + 1 // позиция после <ul…>

		closeAt, ok := findMatchingClose(html, openEnd, "ul")
		if !ok {
			// Несбалансированный — отдаём остаток как есть.
			out.WriteString(html[idx:])
			return out.String()
		}
		ulContent := html[openEnd:closeAt]

		if items, ok := allDirectLIsAreTasks(ulContent); ok {
			out.WriteString(renderTaskList(items, &taskID))
		} else {
			// Не task-list — оставляем оригинал (включая <ul> и </ul>).
			out.WriteString(html[idx : closeAt+5]) // 5 = len("</ul>")
		}
		pos = closeAt + 5
	}
	return out.String()
}

// indexTagOpen ищет в html, начиная с from, следующий открывающий тег
// `<NAME>` или `<NAME ` (с атрибутами). Возвращает индекс `<` или -1.
func indexTagOpen(html string, from int, name string) int {
	needle1 := "<" + name + ">"
	needle2 := "<" + name + " "
	i1 := strings.Index(html[from:], needle1)
	i2 := strings.Index(html[from:], needle2)
	switch {
	case i1 < 0 && i2 < 0:
		return -1
	case i1 < 0:
		return from + i2
	case i2 < 0:
		return from + i1
	case i1 < i2:
		return from + i1
	default:
		return from + i2
	}
}

// findMatchingClose с учётом вложенности находит парный </NAME>.
// from — позиция СРАЗУ после открывающего тега. Возвращает индекс `<` у </NAME>.
func findMatchingClose(html string, from int, name string) (int, bool) {
	openPrefix1 := "<" + name + ">"
	openPrefix2 := "<" + name + " "
	closePrefix := "</" + name + ">"
	depth := 1
	i := from
	for i < len(html) {
		switch {
		case strings.HasPrefix(html[i:], closePrefix):
			depth--
			if depth == 0 {
				return i, true
			}
			i += len(closePrefix)
		case strings.HasPrefix(html[i:], openPrefix1):
			depth++
			i += len(openPrefix1)
		case strings.HasPrefix(html[i:], openPrefix2):
			depth++
			i += len(openPrefix2)
		default:
			i++
		}
	}
	return 0, false
}

// taskItem — распарсенный <li>: статус + текст тела.
type taskItem struct {
	status string // "complete" | "incomplete"
	body   string // XHTML после <input/>
}

// allDirectLIsAreTasks возвращает список тасков, если КАЖДЫЙ прямой <li>
// в ulContent начинается с <input type="checkbox">. Иначе (false, nil).
func allDirectLIsAreTasks(ulContent string) ([]taskItem, bool) {
	var items []taskItem
	pos := 0
	for pos < len(ulContent) {
		idx := indexTagOpen(ulContent, pos, "li")
		if idx < 0 {
			break
		}
		// Между прошлым </li> и новым <li> могут быть только пробелы;
		// если не пробелы — содержимое <ul> чем-то засорено, не задача.
		between := ulContent[pos:idx]
		if strings.TrimSpace(between) != "" {
			return nil, false
		}
		openEnd := strings.Index(ulContent[idx:], ">")
		if openEnd < 0 {
			return nil, false
		}
		openEnd += idx + 1
		closeAt, ok := findMatchingClose(ulContent, openEnd, "li")
		if !ok {
			return nil, false
		}
		body := strings.TrimSpace(ulContent[openEnd:closeAt])
		if !strings.HasPrefix(body, "<input") {
			return nil, false
		}
		// извлекаем тег <input ...> и тело после него.
		gt := strings.Index(body, ">")
		if gt < 0 {
			return nil, false
		}
		inputTag := body[:gt+1]
		if !strings.Contains(inputTag, `type="checkbox"`) {
			return nil, false
		}
		status := "incomplete"
		if strings.Contains(inputTag, "checked") {
			status = "complete"
		}
		items = append(items, taskItem{
			status: status,
			body:   strings.TrimSpace(body[gt+1:]),
		})
		pos = closeAt + 5 // len("</li>")
	}
	// Хвост после последнего </li> должен быть пустым (только whitespace).
	if strings.TrimSpace(ulContent[pos:]) != "" {
		return nil, false
	}
	if len(items) == 0 {
		return nil, false
	}
	return items, true
}

// renderTaskList собирает <ac:task-list> из массива taskItem.
// idCounter — продолжается между разными списками на странице (Confluence
// ждёт уникальные task-id в пределах страницы).
func renderTaskList(items []taskItem, idCounter *int) string {
	var b strings.Builder
	b.WriteString("<ac:task-list>\n")
	for _, it := range items {
		*idCounter++
		fmt.Fprintf(&b,
			"<ac:task><ac:task-id>%d</ac:task-id>"+
				"<ac:task-uuid>%s</ac:task-uuid>"+
				"<ac:task-status>%s</ac:task-status>"+
				"<ac:task-body>%s</ac:task-body></ac:task>\n",
			*idCounter, deterministicUUID(it.body), it.status, it.body,
		)
	}
	b.WriteString("</ac:task-list>")
	return b.String()
}

// deterministicUUID превращает строку в UUID-формат через sha256.
// Не настоящий v4 (не выставляет version-биты), но Confluence формат
// принимает по виду — главное «xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx».
func deterministicUUID(seed string) string {
	h := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(h[0:4]) + "-" +
		hex.EncodeToString(h[4:6]) + "-" +
		hex.EncodeToString(h[6:8]) + "-" +
		hex.EncodeToString(h[8:10]) + "-" +
		hex.EncodeToString(h[10:16])
}
