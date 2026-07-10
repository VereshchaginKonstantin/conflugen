package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// setContentAppearance проставляет внешний вид страницы (full-width|fixed-width)
// через Content Properties API. Confluence хранит draft+published — обновляем оба,
// чтобы изменение сразу видел и редактор, и опубликованная версия.
//
// Алгоритм для одного ключа: GET — если есть, делаем PUT с version+1, иначе POST.
func setContentAppearance(rawAPI rawRequester, baseURL, pageID, value string) error {
	for _, key := range []string{"content-appearance-draft", "content-appearance-published"} {
		if err := setPageProperty(rawAPI, baseURL, pageID, key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	return nil
}

// pagePropertyEnvelope — структура, которой Confluence отдаёт content property.
// Берём только ключевые поля: ключ, значение, версию.
type pagePropertyEnvelope struct {
	Key     string `json:"key,omitempty"`
	Value   string `json:"value"`
	Version *struct {
		Number int `json:"number"`
	} `json:"version,omitempty"`
}

// getPageProperty возвращает (envelope, found, err).
// found=false без err означает 404 — свойство пока не создано.
func getPageProperty(rawAPI rawRequester, baseURL, pageID, key string) (*pagePropertyEnvelope, bool, error) {
	url := fmt.Sprintf("%s/content/%s/property/%s", baseURL, pageID, key)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	body, err := rawAPI.Request(req)
	if err != nil {
		// Confluence отвечает 404 для несуществующего свойства; библиотечный
		// raw transport кладёт код в текст ошибки — этого достаточно.
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
			return nil, false, nil
		}
		return nil, false, err
	}
	var env pagePropertyEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, false, fmt.Errorf("decode property %s: %w", key, err)
	}
	return &env, true, nil
}

func setPageProperty(rawAPI rawRequester, baseURL, pageID, key, value string) error {
	existing, found, err := getPageProperty(rawAPI, baseURL, pageID, key)
	if err != nil {
		return err
	}
	if found && existing.Value == value {
		return nil // уже установлено, пропускаем
	}

	var (
		method string
		url    string
		body   any
	)
	if !found {
		method = http.MethodPost
		url = fmt.Sprintf("%s/content/%s/property", baseURL, pageID)
		body = pagePropertyEnvelope{Key: key, Value: value}
	} else {
		method = http.MethodPut
		url = fmt.Sprintf("%s/content/%s/property/%s", baseURL, pageID, key)
		nextVer := 1
		if existing.Version != nil {
			nextVer = existing.Version.Number + 1
		}
		body = pagePropertyEnvelope{
			Key:   key,
			Value: value,
			Version: &struct {
				Number int `json:"number"`
			}{Number: nextVer},
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if _, err := rawAPI.Request(req); err != nil {
		return fmt.Errorf("%s %s: %w", method, key, err)
	}
	return nil
}
