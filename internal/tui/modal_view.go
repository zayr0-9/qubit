package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m model) renderModal(height int) string {
	if m.modal == nil {
		return ""
	}
	return m.renderModalState(*m.modal, height)
}
func (m model) renderModalState(modal modalState, height int) string {
	if modal.ID == "model_selector" {
		return m.renderModalStateAligned(modal, height, lipgloss.Left, lipgloss.Bottom)
	}
	return m.renderModalStateAligned(modal, height, lipgloss.Center, lipgloss.Bottom)
}
func (m model) renderModalStateAligned(modal modalState, height int, horizontal lipgloss.Position, vertical lipgloss.Position) string {
	panel := m.renderModalPanel(modal, height)
	return m.placeModalPanel(panel, height, horizontal, vertical)
}
func (m model) renderModalPanel(modal modalState, height int) string {
	panelWidth := min(max(48, m.width-12), 92)
	contentWidth := max(20, panelWidth-6)

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Bold(true).Render(modal.Title))
	if modal.Description != "" {
		b.WriteString("\n")
		b.WriteString(wrap(modal.Description, contentWidth))
	}

	if len(modal.Fields) > 0 {
		b.WriteString("\n\n")
		for i, field := range modal.Fields {
			label := mutedSt.Render(field.Label + ":")
			value := field.Value
			if strings.Contains(value, "\n") {
				b.WriteString(label)
				b.WriteString("\n")
				b.WriteString(truncateModalLines(value, contentWidth, 12))
			} else {
				b.WriteString(fmt.Sprintf("%s %s", label, oneLine(value, max(8, contentWidth-lipgloss.Width(field.Label)-2))))
			}
			if i < len(modal.Fields)-1 {
				b.WriteString("\n")
			}
		}
	}

	if len(modal.Options) > 0 {
		b.WriteString("\n\n")
		optionRows := max(1, height-renderedLineCount(b.String())-4)
		window := visibleListWindow(len(modal.Options), modal.OptionCursor, optionRows)
		if window.HasAbove {
			b.WriteString(mutedSt.Render(fmt.Sprintf("  more above (%d)", window.Start)))
			b.WriteString("\n")
		}
		for i := window.Start; i < window.End; i++ {
			option := modal.Options[i]
			marker := "  "
			label := lipgloss.NewStyle().Foreground(cyan).Bold(true).Render(option.Label)
			description := mutedSt.Render(option.Description)
			activeMarker := " "
			if option.Active {
				activeMarker = "•"
			}
			line := fmt.Sprintf("%s%s%s", activeMarker, label, descriptionSuffix(description))
			if i == modal.OptionCursor {
				marker = selectSt.Render("› ")
				line = selectSt.Render(line)
			} else {
				marker = mutedSt.Render(marker)
			}
			b.WriteString(marker + line)
			if i < window.End-1 || window.HasBelow {
				b.WriteString("\n")
			}
		}
		if window.HasBelow {
			b.WriteString(mutedSt.Render(fmt.Sprintf("  more below (%d)", len(modal.Options)-window.End)))
		}
	}

	if len(modal.Actions) > 0 {
		b.WriteString("\n\n")
		for i, action := range modal.Actions {
			button := "[ " + action.Label + " ]"
			if i == modal.Cursor {
				button = selectSt.Render(button)
			} else if action.Style == "danger" {
				button = errSt.Render(button)
			} else {
				button = mutedSt.Render(button)
			}
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(button)
		}
	}

	return lipgloss.NewStyle().Foreground(text).Padding(1, 2).Width(panelWidth).Render(b.String())
}
func (m model) placeModalPanel(panel string, height int, horizontal lipgloss.Position, vertical lipgloss.Position) string {
	return lipgloss.Place(max(1, m.width-4), max(1, height), horizontal, vertical, panel)
}
func descriptionSuffix(description string) string {
	if description == "" {
		return ""
	}
	return "  " + description
}
