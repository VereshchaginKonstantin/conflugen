package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// renderMermaidSVG рендерит mermaid текст в SVG через mmdc (mermaid-cli)
func renderMermaidSVG(content string) (string, error) {
	mmdc, err := exec.LookPath("mmdc")
	if err != nil {
		return "", fmt.Errorf("mmdc не найден: установите @mermaid-js/mermaid-cli (npm install -g @mermaid-js/mermaid-cli): %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "conflugen-mermaid-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputFile := filepath.Join(tmpDir, "diagram.mmd")
	outputFile := filepath.Join(tmpDir, "diagram.svg")

	if err := os.WriteFile(inputFile, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write input: %w", err)
	}

	cmd := exec.Command(mmdc, "-i", inputFile, "-o", outputFile, "-e", "svg", "--quiet")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("mmdc failed: %w\noutput: %s", err, string(out))
	}

	svg, err := os.ReadFile(outputFile)
	if err != nil {
		return "", fmt.Errorf("read svg: %w", err)
	}

	return string(svg), nil
}
