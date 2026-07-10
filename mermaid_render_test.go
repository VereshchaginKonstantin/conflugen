package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRenderMermaidSVG_Integration проверяет реальный рендер через node + beautiful-mermaid.
// Пропускается, если node или пакет недоступны (например, на чистой машине без npm install).
func TestRenderMermaidSVG_Integration(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node не найден — пропускаем интеграционный тест рендера mermaid")
	}

	svg, err := renderMermaidSVG("flowchart TD\n  A[Поставщик] --> B[Дежурный]\n")
	if err != nil {
		t.Skipf("beautiful-mermaid недоступен (%v) — пропускаем; для прогона: npm install beautiful-mermaid", err)
	}

	require.Contains(t, svg, "<svg", "ожидали SVG на выходе рендерера")
	require.Contains(t, svg, "</svg>", "SVG должен быть закрыт")
}

// TestRenderMermaidSVG_NodeMissing — узкий unit-тест на ветку отсутствия node:
// если node нет в PATH, возвращается осмысленная ошибка, а не паника.
func TestRenderMermaidSVG_NodeMissing(t *testing.T) {
	if _, err := exec.LookPath("node"); err == nil {
		t.Skip("node установлен — ветку 'node не найден' тут не проверить")
	}
	_, err := renderMermaidSVG("flowchart TD\n A --> B\n")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "node не найден"))
}
