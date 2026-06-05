package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

func (m model) renderSessionPicker(height int) string {
	sessions := m.sessionPickerSessions()
	panelWidth := max(20, m.width-4)
	contentWidth := max(20, panelWidth-4)
	rowWidth := max(20, contentWidth-2)
	statsWidth := m.sessionPickerStatsWidth(sessions)

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Bold(true).Render("sessions") + "\n")
	if m.sessionSearchMode {
		query := m.sessionSearchQuery
		if query == "" {
			query = mutedSt.Render("type title...")
		}
		b.WriteString(mutedSt.Render("search ") + lipgloss.NewStyle().Foreground(accent).Render(query) + "\n")
		b.WriteString(mutedSt.Render("↑/↓ select · enter activate · esc clear search") + "\n\n")
	} else {
		b.WriteString(mutedSt.Render("↑/↓ select · enter activate · s search · t tree · d delete · esc close") + "\n\n")
	}
	if len(sessions) == 0 {
		if strings.TrimSpace(m.sessionSearchQuery) != "" {
			b.WriteString(mutedSt.Render("no matching sessions"))
		} else {
			b.WriteString(mutedSt.Render("no sessions yet · esc then /new to create one"))
		}
		return lipgloss.NewStyle().Padding(1, 2).Width(panelWidth).Render(b.String())
	}

	maxRows := max(1, height-6)
	if !m.sessionSearchMode {
		maxRows = max(1, height-5)
	}
	window := visibleListWindow(len(sessions), m.sessionCursor, maxRows)
	if window.HasAbove {
		b.WriteString(mutedSt.Render(fmt.Sprintf("  more above (%d)", window.Start)))
		b.WriteString("\n")
	}
	previousHeading := ""
	if window.Start > 0 {
		previousHeading = formatSessionCreatedDateHeading(sessions[window.Start-1].CreatedAt, time.Now())
	}
	for i := window.Start; i < window.End; i++ {
		session := sessions[i]
		heading := formatSessionCreatedDateHeading(session.CreatedAt, time.Now())
		if heading != previousHeading {
			if i > window.Start || window.HasAbove {
				b.WriteString("\n")
			}
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Bold(true).Render(heading))
			b.WriteString("\n")
			previousHeading = heading
		}
		lines := renderSessionPickerRow(session, session.ID == m.session, rowWidth, statsWidth, m.sessionForkCount(session.ID))
		for lineIndex, line := range lines {
			if i == m.sessionCursor {
				marker := "  "
				if lineIndex == 0 {
					marker = "› "
				}
				line = selectSt.Render(marker + line)
			} else {
				line = mutedSt.Render("  ") + line
			}
			b.WriteString(line)
			if lineIndex < len(lines)-1 || i < window.End-1 || window.HasBelow {
				b.WriteString("\n")
			}
		}
	}
	if window.HasBelow {
		b.WriteString(mutedSt.Render(fmt.Sprintf("  more below (%d)", len(sessions)-window.End)))
	}
	return lipgloss.NewStyle().Padding(1, 2).Width(panelWidth).Render(b.String())
}
func (m model) sessionPickerStatsWidth(sessions []sessionInfo) int {
	statsWidth := lipgloss.Width(formatSessionPickerStats(sessionInfo{}, 0))
	for _, session := range sessions {
		statsWidth = max(statsWidth, lipgloss.Width(formatSessionPickerStats(session, m.sessionForkCount(session.ID))))
	}
	return statsWidth
}
func (m model) sessionForkCount(sessionID string) int {
	if sessionID == "" {
		return 0
	}
	count := 0
	for _, session := range m.sessions {
		if session.ForkedFromSessionID == "" || session.ID == sessionID {
			continue
		}
		seen := map[string]bool{session.ID: true}
		for parentID := session.ForkedFromSessionID; parentID != ""; parentID = m.parentSessionID(parentID) {
			if parentID == sessionID {
				count++
				break
			}
			if seen[parentID] {
				break
			}
			seen[parentID] = true
		}
	}
	return count
}
func (m model) parentSessionID(sessionID string) string {
	for _, session := range m.sessions {
		if session.ID == sessionID {
			return session.ForkedFromSessionID
		}
	}
	return ""
}
func renderSessionPickerRow(session sessionInfo, active bool, width int, statsWidth int, forkCount int) []string {
	activeMarker := " "
	if active {
		activeMarker = "•"
	}
	favouriteMarker := " "
	if strings.TrimSpace(session.FavouritedAt) != "" {
		favouriteMarker = "★"
	}
	prefix := activeMarker + " " + favouriteMarker
	stats := formatSessionPickerStats(session, forkCount)
	statsWidth = max(statsWidth, lipgloss.Width(stats))
	titleWidth := max(1, width-lipgloss.Width(prefix)-statsWidth-3)
	if titleWidth+lipgloss.Width(prefix)+statsWidth+3 > width {
		stats = oneLine(stats, max(1, width-lipgloss.Width(prefix)-4))
		statsWidth = lipgloss.Width(stats)
		titleWidth = max(1, width-lipgloss.Width(prefix)-statsWidth-3)
	}
	titleLines := sessionPickerTitleLines(session.Title, titleWidth)
	firstTitle := ""
	if len(titleLines) > 0 {
		firstTitle = titleLines[0]
	}
	lines := []string{fmt.Sprintf("%s %-*s %*s", prefix, titleWidth, firstTitle, statsWidth, stats)}
	if len(titleLines) > 1 {
		continuationPrefix := strings.Repeat(" ", lipgloss.Width(prefix))
		lines = append(lines, fmt.Sprintf("%s %-*s %*s", continuationPrefix, titleWidth, titleLines[1], statsWidth, ""))
	}
	return lines
}

func sessionPickerTitleLines(title string, width int) []string {
	cleaned := strings.Join(strings.Fields(title), " ")
	if cleaned == "" {
		cleaned = "New chat"
	}
	if width <= 0 {
		return []string{cleaned}
	}
	if lipgloss.Width(cleaned) <= width {
		return []string{cleaned}
	}
	runes := []rune(cleaned)
	firstCut := min(len(runes), max(0, width-1))
	first := oneLine(cleaned, width)
	remaining := strings.TrimSpace(string(runes[firstCut:]))
	if remaining == "" {
		return []string{first}
	}
	return []string{first, oneLine(remaining, width)}
}
func formatSessionPickerStats(session sessionInfo, forkCount int) string {
	parts := []string{formatSessionCreatedAt(session.CreatedAt)}
	parts = append(parts, fmt.Sprintf("%d %s", session.MessageCount, pluralLabel(session.MessageCount, "msg", "msgs")))
	parts = append(parts, fmt.Sprintf("%d %s", forkCount, pluralLabel(forkCount, "fork", "forks")))
	return strings.Join(parts, " · ")
}
func formatSessionCreatedAt(createdAt string) string {
	if t, ok := parseSessionTime(createdAt); ok {
		return t.Format("15:04")
	}
	return "--:--"
}

func formatSessionCreatedDateHeading(createdAt string, now time.Time) string {
	if t, ok := parseSessionTime(createdAt); ok {
		createdDate := dateOnly(t)
		today := dateOnly(now.Local())
		switch {
		case createdDate.Equal(today):
			return "Today"
		case createdDate.Equal(today.AddDate(0, 0, -1)):
			return "Yesterday"
		default:
			return fmt.Sprintf("%d %s", t.Day(), t.Format("January"))
		}
	}
	return "Unknown date"
}

func parseSessionTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.Local(), true
	}
	if t, err := time.Parse("2006-01-02T15:04:05", value); err == nil {
		return t.Local(), true
	}
	return time.Time{}, false
}

func dateOnly(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}
func pluralLabel(count int, singular string, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
