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
	for i := window.Start; i < window.End; i++ {
		session := sessions[i]
		line := renderSessionPickerRow(session, session.ID == m.session, rowWidth, statsWidth, m.sessionForkCount(session.ID))
		if i == m.sessionCursor {
			line = selectSt.Render("  " + line)
		} else {
			line = mutedSt.Render("  ") + line
		}
		b.WriteString(line)
		if i < window.End-1 || window.HasBelow {
			b.WriteString("\n")
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
func renderSessionPickerRow(session sessionInfo, active bool, width int, statsWidth int, forkCount int) string {
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
	title := oneLine(session.Title, titleWidth)
	return fmt.Sprintf("%s %-*s %*s", prefix, titleWidth, title, statsWidth, stats)
}
func formatSessionPickerStats(session sessionInfo, forkCount int) string {
	parts := []string{formatSessionCreatedAt(session.CreatedAt)}
	parts = append(parts, fmt.Sprintf("%d %s", session.MessageCount, pluralLabel(session.MessageCount, "msg", "msgs")))
	parts = append(parts, fmt.Sprintf("%d %s", forkCount, pluralLabel(forkCount, "fork", "forks")))
	return strings.Join(parts, " · ")
}
func formatSessionCreatedAt(createdAt string) string {
	createdAt = strings.TrimSpace(createdAt)
	if createdAt == "" {
		return "-- -- --:--"
	}
	if t, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
		return t.Local().Format("02-01 15:04")
	}
	if t, err := time.Parse("2006-01-02T15:04:05", createdAt); err == nil {
		return t.Local().Format("02-01 15:04")
	}
	return oneLine(createdAt, len("02-01 15:04"))
}
func pluralLabel(count int, singular string, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
