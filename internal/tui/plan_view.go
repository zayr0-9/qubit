package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func (m *model) renderPlanViewMessage(message chatMessage, title string, width int) string {
	panelWidth := min(max(44, width), 96)
	contentWidth := max(20, panelWidth-6)
	header := aiIcon.Render("◇") + " " + lipgloss.NewStyle().Foreground(accent).Bold(true).Render(oneLine(title, contentWidth))
	if message.Path != "" {
		header += mutedSt.Render(" · " + oneLine(message.Path, max(12, contentWidth-lipgloss.Width(stripANSI(title))-3)))
	}
	content, err := renderMessageContentAtWidth(chatMessage{Role: "assistant", Content: message.Content}, contentWidth)
	if err != nil {
		content = wrap(message.Content, contentWidth)
	}
	body := header
	if strings.TrimSpace(content) != "" {
		body += "\n" + strings.Join(strings.Split(content, "\n"), "\n")
	}
	return renderAccentBorderedPanel(body, panelWidth)
}
