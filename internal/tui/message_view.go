package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m *model) refreshViewport() {
	previousYOffset := m.viewport.YOffset()
	m.toolHitboxes = nil
	m.linkHitboxes = nil
	m.transcriptLines = nil
	var b strings.Builder
	contentLine := 0
	for i := 0; i < len(m.messages); i++ {
		message := m.messages[i]
		if i > 0 {
			separator := messageSeparator(m.messages[i-1], message)
			b.WriteString(separator)
			contentLine += separatorBlankLineCount(separator)
		}
		if message.Role == "view" {
			rendered := m.renderViewMessage(message)
			b.WriteString(rendered)
			contentLine += renderedLineCount(rendered)
			continue
		}
		if message.Role == "tool" {
			startLine := contentLine
			groups := []*toolGroup{}
			for i < len(m.messages) && m.messages[i].Role == "tool" {
				if m.messages[i].ToolGroup != nil {
					groups = append(groups, m.messages[i].ToolGroup)
				}
				i++
			}
			i--
			rendered := m.renderCollapsedToolGroups(groups, max(20, m.viewport.Width()))
			b.WriteString(rendered)
			m.appendToolGroupHitboxes(groups, startLine, max(20, m.viewport.Width()))
			contentLine += renderedLineCount(rendered)
			continue
		}
		if message.Role == "reasoning" {
			rendered := m.renderReasoningBlock(message, max(20, m.viewport.Width()))
			b.WriteString(rendered)
			headingWidth := lipgloss.Width(m.renderReasoningBlockHeading(message))
			m.toolHitboxes = append(m.toolHitboxes, toolHitbox{Kind: "reasoning", MessageIndex: i, StartY: contentLine, EndY: contentLine, StartX: 0, EndX: max(0, headingWidth-1)})
			contentLine += renderedLineCount(rendered)
			continue
		}
		cacheable := !(m.streaming && i == m.streamingMessageIndex)
		rendered := renderMessageWithIcon(message, m.renderMessageContent(message, cacheable), messageDisplayNumber(m.messages, i))
		b.WriteString(rendered)
		contentLine += renderedLineCount(rendered)
		if message.Role == "user" && i == len(m.messages)-1 {
			b.WriteString("\n")
			contentLine++
		}
	}
	content := b.String()
	m.transcriptContent = content
	m.transcriptLines = transcriptRenderLines(content)
	m.linkHitboxes = transcriptLinkHitboxes(m.transcriptLines)
	m.repaintTranscriptSelection()
	m.restoreViewportPosition(previousYOffset)
}
func (m *model) restoreViewportPosition(yOffset int) {
	if m.autoScroll {
		m.viewport.GotoBottom()
		return
	}
	m.viewport.SetYOffset(clampInt(yOffset, 0, max(0, m.viewport.TotalLineCount()-m.viewport.Height())))
}
func messageSeparator(prev chatMessage, next chatMessage) string {
	if prev.Role == "user" {
		return "\n\n"
	}
	if prev.Role == "tool" || next.Role == "tool" {
		return "\n"
	}
	if prev.Role == next.Role {
		return "\n"
	}
	return "\n\n"
}
func separatorBlankLineCount(separator string) int {
	newlines := strings.Count(separator, "\n")
	if newlines == 0 {
		return 0
	}
	return newlines - 1
}
func (m *model) renderViewMessage(message chatMessage) string {
	title := fallback(message.Title, "View")
	isPlan := message.ViewType == "plan"
	if isPlan && !strings.HasPrefix(title, "Plan:") {
		title = "Plan: " + title
	}

	width := max(20, m.viewport.Width()-2)
	if isPlan {
		return m.renderPlanViewMessage(message, title, width)
	}

	header := aiIcon.Render("◇") + " " + lipgloss.NewStyle().Foreground(accent).Bold(true).Render(title)
	if message.Path != "" {
		header += mutedSt.Render(" · " + oneLine(message.Path, max(12, m.viewport.Width()-lipgloss.Width(title)-8)))
	}
	content, err := renderMessageContentAtWidth(chatMessage{Role: "assistant", Content: message.Content}, width)
	if err != nil {
		content = wrap(message.Content, width)
	}
	if strings.TrimSpace(content) == "" {
		return header
	}
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	return header + "\n" + strings.Join(lines, "\n")
}
func renderAccentBorderedPanel(body string, width int) string {
	return lipgloss.NewStyle().
		Foreground(text).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(0, 2).
		Width(width).
		Render(body)
}
func renderMessageWithIcon(message chatMessage, content string, number int) string {
	icon := aiIcon.Render("◆")
	if message.LocalOnly || message.Role == "status" {
		icon = mutedSt.Render("◇")
	} else if message.Role == "reasoning" {
		icon = mutedSt.Render("◇")
	} else if message.Role == "user" {
		if number > 0 {
			icon = userIcon.Render(fmt.Sprintf("›%d", number))
		} else {
			icon = userIcon.Render("›")
		}
	} else if message.Role == "error" {
		icon = errorIcon.Render("!")
	}

	if content == "" {
		return icon
	}

	lines := strings.Split(content, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) == 0 {
		return icon
	}
	indent := strings.Repeat(" ", max(2, lipgloss.Width(icon)+1))
	lines[0] = icon + " " + strings.TrimLeft(lines[0], " \t")
	for i := 1; i < len(lines); i++ {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}
func messageDisplayNumber(messages []chatMessage, index int) int {
	if index < 0 || index >= len(messages) || messages[index].Role != "user" {
		return 0
	}
	return index + 1
}
func (m *model) renderMessageContent(message chatMessage, cacheable bool) string {
	width := max(20, m.viewport.Width())
	key := renderCacheKey{Role: message.Role, Content: message.Content, ReasoningContent: message.ReasoningContent, Width: width}
	if cacheable && m.renderCache != nil {
		if cached, ok := m.renderCache[key]; ok {
			return cached
		}
	}

	rendered, err := renderMessageContentAtWidth(message, width)
	if err != nil {
		rendered = wrap(message.Content, width)
	}
	if cacheable && m.renderCache != nil {
		m.renderCache[key] = rendered
	}
	return rendered
}
