package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

// uploadMermaidDiagrams рендерит и загружает mermaid диаграммы как вложения к странице
func uploadMermaidDiagrams(
	rawAPI rawRequester,
	baseURL string,
	pageID string,
	diagrams []extensions.MermaidDiagram,
) error {
	for _, d := range diagrams {
		// Загружаем исходник
		if err := uploadAttachment(rawAPI, baseURL, pageID, d.Filename, d.Content); err != nil {
			return fmt.Errorf("upload mermaid source %s: %w", d.Filename, err)
		}

		// Рендерим SVG через mmdc
		svg, err := renderMermaidSVG(d.Content)
		if err != nil {
			log.Printf("  предупреждение: не удалось отрендерить %s: %v", d.Filename, err)
			log.Printf("  диаграмма загружена без SVG — в Confluence может не отобразиться")
			continue
		}

		// Загружаем SVG
		svgFilename := d.Filename + ".svg"
		if err := uploadAttachment(rawAPI, baseURL, pageID, svgFilename, svg); err != nil {
			return fmt.Errorf("upload mermaid svg %s: %w", svgFilename, err)
		}

		log.Printf("  загружена диаграмма: %s (+svg)", d.Filename)
	}
	return nil
}

// attachmentSearchResult — ответ поиска вложений
type attachmentSearchResult struct {
	Results []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"results"`
}

// findExistingAttachment ищет вложение по имени файла
func findExistingAttachment(
	rawAPI rawRequester,
	baseURL string,
	pageID string,
	filename string,
) (string, error) {
	url := fmt.Sprintf("%s/content/%s/child/attachment?filename=%s", baseURL, pageID, filename)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := rawAPI.Request(req)
	if err != nil {
		return "", err
	}

	var result attachmentSearchResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", err
	}

	for _, att := range result.Results {
		if att.Title == filename {
			return att.ID, nil
		}
	}

	return "", nil
}

// uploadAttachment загружает файл как вложение к странице через Confluence REST API
func uploadAttachment(
	rawAPI rawRequester,
	baseURL string,
	pageID string,
	filename string,
	content string,
) error {
	body, contentType, err := buildMultipartBody(filename, content)
	if err != nil {
		return err
	}

	existingID, _ := findExistingAttachment(rawAPI, baseURL, pageID, filename)

	var url string
	if existingID != "" {
		url = fmt.Sprintf("%s/content/%s/child/attachment/%s/data", baseURL, pageID, existingID)
	} else {
		url = fmt.Sprintf("%s/content/%s/child/attachment", baseURL, pageID)
	}

	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Atlassian-Token", "nocheck")

	_, err = rawAPI.Request(req)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "409") {
			return updateExistingAttachment(rawAPI, baseURL, pageID, filename, content)
		}
		return fmt.Errorf("upload request: %w", err)
	}

	return nil
}

// updateExistingAttachment обновляет существующее вложение
func updateExistingAttachment(
	rawAPI rawRequester,
	baseURL string,
	pageID string,
	filename string,
	content string,
) error {
	attID, err := findExistingAttachment(rawAPI, baseURL, pageID, filename)
	if err != nil || attID == "" {
		return fmt.Errorf("find existing attachment %s: %w", filename, err)
	}

	body, contentType, err := buildMultipartBody(filename, content)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/content/%s/child/attachment/%s/data", baseURL, pageID, attID)
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Atlassian-Token", "nocheck")

	_, err = rawAPI.Request(req)
	if err != nil {
		return fmt.Errorf("update attachment request: %w", err)
	}

	return nil
}

// buildMultipartBody создаёт multipart body с файлом
func buildMultipartBody(filename, content string) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", fmt.Errorf("create form file: %w", err)
	}

	if _, err := io.WriteString(part, content); err != nil {
		return nil, "", fmt.Errorf("write content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("close writer: %w", err)
	}

	return &buf, writer.FormDataContentType(), nil
}
