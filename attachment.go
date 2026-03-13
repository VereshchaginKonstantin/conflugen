package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"

	"github.com/VereshchaginKonstantin/conflugen/extensions"
)

// uploadMermaidDiagrams загружает mermaid диаграммы как вложения к странице
func uploadMermaidDiagrams(
	rawAPI rawRequester,
	baseURL string,
	pageID string,
	diagrams []extensions.MermaidDiagram,
) error {
	for _, d := range diagrams {
		if err := uploadAttachment(rawAPI, baseURL, pageID, d.Filename, d.Content); err != nil {
			return fmt.Errorf("upload mermaid attachment %s: %w", d.Filename, err)
		}
		log.Printf("  загружена диаграмма: %s", d.Filename)
	}
	return nil
}

// uploadAttachment загружает файл как вложение к странице через Confluence REST API
func uploadAttachment(
	rawAPI rawRequester,
	baseURL string,
	pageID string,
	filename string,
	content string,
) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}

	if _, err := io.WriteString(part, content); err != nil {
		return fmt.Errorf("write content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close writer: %w", err)
	}

	url := fmt.Sprintf("%s/content/%s/child/attachment", baseURL, pageID)
	req, err := http.NewRequest(http.MethodPut, url, &buf)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Atlassian-Token", "nocheck")

	_, err = rawAPI.Request(req)
	if err != nil {
		return fmt.Errorf("upload request: %w", err)
	}

	return nil
}
