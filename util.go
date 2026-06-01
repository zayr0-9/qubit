package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func wrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out []string
	for _, paragraph := range strings.Split(s, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range words {
			if len(line) == 0 {
				line = word
			} else if len([]rune(line))+1+len([]rune(word)) <= width {
				line += " " + word
			} else {
				out = append(out, line)
				line = word
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func short(s string, n int) string {
	if s == "" || len([]rune(s)) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n]) + "…"
}

func oneLine(s string, width int) string {
	s = strings.Join(strings.Fields(s), " ")
	if width <= 0 || len([]rune(s)) <= width {
		return s
	}
	runes := []rune(s)
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func fallback(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func findAppRoot() (string, error) {
	candidates := []string{}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, exeDir, filepath.Dir(exeDir))
	}

	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		if isAppRoot(abs) {
			return abs, nil
		}
	}

	return "", fmt.Errorf("could not find Qubit app root. Run from D:\\qubit or keep bin\\qubit.exe under the project root")
}

func isAppRoot(dir string) bool {
	if hasFile(dir, "package.json") && hasFile(dir, "go.mod") {
		return true
	}
	if hasFile(dir, filepath.Join("dist", "runtime.js")) {
		return true
	}
	if hasFile(dir, "runtime.ts") {
		return true
	}
	return hasFile(dir, "runtime.mjs")
}

func hasFile(dir, name string) bool {
	info, err := os.Stat(filepath.Join(dir, name))
	return err == nil && !info.IsDir()
}

func renderedLineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
