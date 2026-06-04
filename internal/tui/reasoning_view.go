package tui

import "strings"

func (m *model) renderReasoningBlock(message chatMessage, width int) string {
	heading := m.renderReasoningBlockHeading(message)
	if !message.Expanded {
		return heading
	}
	content := strings.TrimSpace(stripReasoningTitle(message.Content))
	if content == "" {
		return heading
	}
	lines := strings.Split(wrap(content, max(20, width-4)), "\n")
	for i := range lines {
		lines[i] = reasoningSt.Render("  " + lines[i])
	}
	return heading + "\n" + strings.Join(lines, "\n")
}
func (m *model) renderReasoningBlockHeading(message chatMessage) string {
	icon := "✦"
	title := reasoningBlockTitle(message.Content)
	return reasoningSt.Render(icon) + " " + reasoningSt.Bold(true).Render(title)
}
func reasoningBlockTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		title, ok := boldMarkdownLineTitle(line)
		if ok {
			return title
		}
	}
	preview := oneLine(strings.TrimSpace(stripReasoningTitle(content)), 56)
	if preview == "" {
		return "Thinking"
	}
	return preview
}
func boldMarkdownLineTitle(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "**") {
		return "", false
	}
	rest := strings.TrimPrefix(trimmed, "**")
	end := strings.Index(rest, "**")
	if end < 0 {
		return "", false
	}
	title := strings.TrimSpace(rest[:end])
	return title, title != ""
}
func stripReasoningTitle(content string) string {
	lines := strings.Split(content, "\n")
	for len(lines) > 0 {
		if _, ok := boldMarkdownLineTitle(lines[0]); ok {
			lines = lines[1:]
			for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
				lines = lines[1:]
			}
			break
		}
		if strings.TrimSpace(lines[0]) != "" {
			break
		}
		lines = lines[1:]
	}
	return strings.Join(lines, "\n")
}
