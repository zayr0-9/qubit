package tui

import "strings"

func renderChat(content string, width int, height int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	leftPad := strings.Repeat(" ", max(0, chatStyle.GetPaddingLeft()))
	for i, line := range lines {
		lines[i] = leftPad + line
	}
	return strings.Join(lines, "\n")
}

func renderFixedHeight(content string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
