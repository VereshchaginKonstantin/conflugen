package main

import (
	"fmt"
	"log"

	goconfluence "github.com/virtomize/confluence-go-api"
)

// syncLabels приводит набор меток страницы pageID к desired:
// удаляет те, которых нет в desired, добавляет недостающие.
// Сравнение по полю Label.Name; префикс используется global.
// Пустой desired и пустые текущие метки → no-op.
func syncLabels(api confluenceAPI, pageID string, desired []string) error {
	current, err := api.GetLabels(pageID)
	if err != nil {
		return fmt.Errorf("GetLabels(%s): %w", pageID, err)
	}

	have := map[string]struct{}{}
	if current != nil {
		for _, l := range current.Labels {
			have[l.Name] = struct{}{}
		}
	}
	want := map[string]struct{}{}
	for _, l := range desired {
		want[l] = struct{}{}
	}

	var toAdd []goconfluence.Label
	for name := range want {
		if _, ok := have[name]; !ok {
			toAdd = append(toAdd, goconfluence.Label{Prefix: "global", Name: name})
		}
	}

	var toRemove []string
	for name := range have {
		if _, ok := want[name]; !ok {
			toRemove = append(toRemove, name)
		}
	}

	if len(toAdd) > 0 {
		if _, err := api.AddLabels(pageID, &toAdd); err != nil {
			return fmt.Errorf("AddLabels(%s): %w", pageID, err)
		}
		log.Printf("  +метки: %v", labelNames(toAdd))
	}
	for _, name := range toRemove {
		if _, err := api.DeleteLabel(pageID, name); err != nil {
			return fmt.Errorf("DeleteLabel(%s,%s): %w", pageID, name, err)
		}
		log.Printf("  -метка: %s", name)
	}

	return nil
}

func labelNames(labels []goconfluence.Label) []string {
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names
}
