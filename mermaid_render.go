package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// beautifulMermaidScript — ESM-скрипт для `node --input-type=module -e`.
// Читает mermaid-текст из stdin, рендерит его в SVG через пакет beautiful-mermaid
// (чистый JS на elkjs, без браузера/Chromium) и пишет SVG в stdout.
//
// Резолв пакета — через import.meta.resolve относительно текущей рабочей директории
// (createRequire не годится: у beautiful-mermaid "exports" без CJS-условия). Поэтому
// достаточно `npm install beautiful-mermaid` в каталоге, откуда запускается conflugen
// (либо глобальной установки — import.meta.resolve учитывает её при наличии в путях).
const beautifulMermaidScript = `
import { readFileSync } from 'node:fs';
const url = await import.meta.resolve('beautiful-mermaid');
const bm = await import(url);
const svg = await bm.renderMermaidSVGAsync(readFileSync(0, 'utf8'));
process.stdout.write(svg);
`

// renderMermaidSVG рендерит mermaid-текст в SVG через beautiful-mermaid (node, без Chromium).
func renderMermaidSVG(content string) (string, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return "", fmt.Errorf("node не найден: установите Node.js и пакет beautiful-mermaid (npm install beautiful-mermaid): %w", err)
	}

	cmd := exec.Command(node, "--input-type=module", "-e", beautifulMermaidScript)
	cmd.Stdin = strings.NewReader(content)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("beautiful-mermaid render failed (установлен ли пакет? `npm install beautiful-mermaid`): %w\n%s", err, stderr.String())
	}

	svg := stdout.String()
	if !strings.Contains(svg, "<svg") {
		return "", fmt.Errorf("beautiful-mermaid вернул не-SVG: %s", truncateForError(svg, 200))
	}
	return svg, nil
}

func truncateForError(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
