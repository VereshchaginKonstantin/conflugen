package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// crawlItem — элемент очереди обхода: ссылка плюс от кого мы к ней пришли
// (нужно для рёбер в index.json).
type crawlItem struct {
	ref  pageRef
	from string // pageID источника; пусто для корневых
	kind string // "root" | "child" | "link"
}

// crawler обходит Confluence в ширину, складывая страницы на диск. Единственное
// место, где встречаются сеть (pageSource) и диск (pageStore).
//
// Обход не ограничен ни глубиной, ни числом страниц — так задумано. Штатный
// способ остановиться — Ctrl+C, который отменяет контекст.
type crawler struct {
	src   *pageSource
	store *pageStore
	// selfHost — хост нашего Confluence: ссылки на другие хосты не считаются
	// ссылками на страницы.
	selfHost string
	// force — перекачивать страницы, уже выгруженные в актуальной версии.
	force bool

	visited map[string]bool
	done    int
}

func newCrawler(src *pageSource, store *pageStore, selfHost string, force bool) *crawler {
	return &crawler{
		src:      src,
		store:    store,
		selfHost: selfHost,
		force:    force,
		visited:  make(map[string]bool),
	}
}

// Crawl обходит граф от корневых pageID вширь. Возвращает ошибку только на
// фатальном: неверные креды либо невозможность писать на диск. Отказ отдельной
// страницы (403/404) фиксируется в index.json и обход продолжается.
func (c *crawler) Crawl(ctx context.Context, rootIDs []string) error {
	queue := make([]crawlItem, 0, len(rootIDs))
	for _, id := range rootIDs {
		queue = append(queue, crawlItem{ref: pageRef{ID: id}, kind: "root"})
	}

	for len(queue) > 0 {
		if ctx.Err() != nil {
			log.Printf("обход прерван: %d страниц скачано, %d в очереди", c.done, len(queue))
			return nil
		}

		item := queue[0]
		queue = queue[1:]

		pageID, err := c.src.ResolveRef(item.ref)
		if err != nil {
			if errors.Is(err, errAuth) {
				return err
			}
			log.Printf("предупреждение: не удалось резолвить %s: %v", item.ref.Key(), err)
			continue
		}
		if pageID == "" {
			log.Printf("предупреждение: страница %s не найдена — пропускаю", item.ref.Key())
			continue
		}

		c.store.RecordEdge(item.from, pageID, item.kind)

		// Помечаем ДО запроса. Иначе страница, упавшая с 403, вернётся в
		// очередь по следующей ссылке на неё — при обходе без лимитов это
		// вечный цикл.
		if c.visited[pageID] {
			log.Printf("  ∘ %s уже обойдена (пришли по %s от %s) — пропускаю",
				pageID, item.kind, sourceOf(item.from))
			continue
		}
		c.visited[pageID] = true

		// Печатаем ДО запроса: при интервале в секунды пользователь иначе
		// видит молчание и не понимает, работает ли программа.
		log.Printf("→ [%d] запрашиваю %s (пришли по %s от %s; в очереди ещё %d)",
			c.done+1, pageID, item.kind, sourceOf(item.from), len(queue))

		next, err := c.processPage(ctx, pageID)
		if err != nil {
			// Фатальная ошибка (диск, 401) — прекращаем весь обход.
			return err
		}

		newLinks, newChildren := countKinds(next)
		if len(next) > 0 {
			log.Printf("  ⇒ добавлено в очередь: %d по ссылкам, %d дочерних (очередь: %d → %d)",
				newLinks, newChildren, len(queue), len(queue)+len(next))
		}
		queue = append(queue, next...)

		if err := c.store.FlushIndex(); err != nil {
			return err
		}
	}

	log.Printf("обход завершён: %d страниц скачано, %d уникальных id посещено", c.done, len(c.visited))
	return nil
}

// sourceOf — человекочитаемый источник ребра для лога.
func sourceOf(from string) string {
	if from == "" {
		return "списка --list"
	}
	return from
}

// countKinds считает, сколько рёбер каждого вида добавляется в очередь.
func countKinds(items []crawlItem) (links, children int) {
	for _, it := range items {
		if it.kind == "child" {
			children++
			continue
		}
		links++
	}
	return links, children
}

// processPage выгружает одну страницу и возвращает найденные на ней рёбра.
// Отказ самой страницы (403/404) наверх не возвращается: он пишется в
// index.json, а обход продолжается. Наверх идут только фатальные ошибки —
// неверные креды (errAuth) и невозможность писать на диск (errDiskWrite).
func (c *crawler) processPage(ctx context.Context, pageID string) ([]crawlItem, error) {
	page, err := c.src.GetPage(pageID)
	if err != nil {
		// Креды не приняты — следующие страницы упадут так же. Останавливаемся,
		// а не заполняем index.json сотней одинаковых ошибок.
		if errors.Is(err, errAuth) {
			return nil, err
		}
		log.Printf("предупреждение: страница %s не получена: %v", pageID, err)
		c.store.RecordPage(indexPage{ID: pageID, Error: err.Error()})
		return nil, nil
	}

	log.Printf("  ← «%s» %s, space=%s, тело %d байт",
		page.Meta.Title, versionOf(page.Meta.Version), page.Meta.SpaceKey, len(page.Storage))

	skipped := false
	if !c.force && c.store.IsComplete(page.Meta.ID, page.Meta.Title, page.Meta.Version) {
		skipped = true
		log.Printf("  ✓ уже выгружена в этой версии — файлы и вложения не трогаю")
	} else if err := c.savePage(ctx, &page); err != nil {
		if errors.Is(err, errDiskWrite) || errors.Is(err, errAuth) {
			return nil, err
		}
		log.Printf("предупреждение: страница %s выгружена не полностью: %v", pageID, err)
		c.store.RecordPage(indexPage{ID: pageID, Title: page.Meta.Title, Error: err.Error()})
		// page.json не записан — resume перекачает страницу. Ссылки всё равно
		// обходим: тело мы уже получили.
		return c.linksFrom(page), nil
	}

	c.done++
	c.store.RecordPage(indexPage{
		ID:      page.Meta.ID,
		Title:   page.Meta.Title,
		Dir:     pageDirName(page.Meta.ID, page.Meta.Title),
		Version: page.Meta.Version,
		Skipped: skipped,
	})

	// Ссылки с пропущенной по resume страницы всё равно идут в очередь: сама
	// она на диске, но её рёбра ещё не обойдены.
	return c.linksFrom(page), nil
}

// savePage пишет тело, вложения и — последним — page.json. Если хоть одно
// вложение не скачалось, page.json не пишется: страница неполна, и следующий
// запуск перекачает её.
func (c *crawler) savePage(ctx context.Context, page *fetchedPage) error {
	m := &page.Meta

	if err := c.store.SaveXHTML(m.ID, m.Title, page.Storage); err != nil {
		return err
	}

	atts, err := c.src.ListAttachments(m.ID)
	if err != nil {
		return err
	}
	if len(atts) > 0 {
		log.Printf("  вложений: %d", len(atts))
	}

	for i, att := range atts {
		// Ctrl+C посреди списка вложений: выходим, не записав page.json, —
		// страница останется помеченной как неполная и перекачается при resume.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		name := c.store.AttachmentName(m.ID, att.Title)

		data, err := c.src.DownloadAttachment(att.DownloadPath)
		if err != nil {
			return err
		}
		if err := c.store.SaveAttachment(m.ID, m.Title, name, data); err != nil {
			return err
		}
		m.Attachments = append(m.Attachments, name)
		log.Printf("    ↓ [%d/%d] %s — %s", i+1, len(atts), name, humanSize(len(data)))
	}

	m.DownloadedAt = time.Now().UTC().Format(time.RFC3339)

	// page.json — последним. Его наличие и есть маркер полноты.
	if err := c.store.SaveMeta(*m); err != nil {
		return err
	}

	log.Printf("  ✓ [%d] сохранено в %s/ (+%d вложений)",
		c.done+1, pageDirName(m.ID, m.Title), len(m.Attachments))
	return nil
}

// humanSize форматирует размер для лога: 1.2 МБ читается быстрее, чем 1258291.
func humanSize(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d Б", n)
	}
	div, exp := int64(unit), 0
	for size := int64(n) / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cБ", float64(n)/float64(div), "КМГТ"[exp])
}

// linksFrom собирает рёбра страницы: сначала ссылки из тела, потом дочерние.
// Недоступный список детей — не повод ронять обход: возвращаем то, что есть.
func (c *crawler) linksFrom(page fetchedPage) []crawlItem {
	var out []crawlItem

	for _, ref := range extractLinks(page.Storage, page.Meta.SpaceKey, c.selfHost) {
		out = append(out, crawlItem{ref: ref, from: page.Meta.ID, kind: "link"})
	}

	children, err := c.src.ListChildren(page.Meta.ID)
	if err != nil {
		log.Printf("предупреждение: не удалось получить дочерние страницы %s: %v", page.Meta.ID, err)
		return out
	}
	for _, childID := range children {
		out = append(out, crawlItem{ref: pageRef{ID: childID}, from: page.Meta.ID, kind: "child"})
	}

	return out
}
