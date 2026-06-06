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

	header := m.renderModalHeader(modal, contentWidth)
	body := m.renderModalBody(modal, contentWidth, height)

	var b strings.Builder
	b.WriteString(header)
	if body != "" {
		b.WriteString("\n\n")
		b.WriteString(body)
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

func (m model) renderModalHeader(modal modalState, contentWidth int) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Bold(true).Render(modal.Title))
	if modal.Description != "" {
		b.WriteString("\n")
		b.WriteString(wrap(modal.Description, contentWidth))
	}
	return b.String()
}

func (m model) renderModalBody(modal modalState, contentWidth int, height int) string {
	lines := m.modalContentLinesFor(modal, contentWidth)
	if len(lines) == 0 {
		return ""
	}
	visibleRows := m.modalScrollableRowsForHeight(modal, height)
	if visibleRows >= len(lines) {
		return strings.Join(lines, "\n")
	}
	offset := min(max(modal.ScrollOffset, 0), max(0, len(lines)-visibleRows))
	visible := append([]string{}, lines[offset:offset+visibleRows]...)
	if offset > 0 {
		visible[0] = mutedSt.Render(fmt.Sprintf("↑ more above (%d)", offset))
	}
	if below := len(lines) - offset - visibleRows; below > 0 {
		if offset > 0 && len(visible) == 1 {
			visible[0] = mutedSt.Render(fmt.Sprintf("↑ more above (%d) · ↓ more below (%d)", offset, below))
		} else {
			visible[len(visible)-1] = mutedSt.Render(fmt.Sprintf("↓ more below (%d)", below))
		}
	}
	return strings.Join(visible, "\n")
}

func (m model) modalScrollableRows(modal modalState) int {
	return m.modalScrollableRowsForHeight(modal, m.modalPanelAvailableHeight())
}

func (m model) modalScrollableRowsForHeight(modal modalState, height int) int {
	panelHeight := max(1, height)
	headerRows := renderedLineCount(m.renderModalHeader(modal, max(20, min(max(48, m.width-12), 92)-6)))
	actionRows := 0
	if len(modal.Actions) > 0 {
		actionRows = 3
	}
	bodySeparatorRows := 0
	if len(modal.Fields) > 0 {
		bodySeparatorRows = 2
	}
	paddingRows := 2
	return max(1, panelHeight-headerRows-actionRows-bodySeparatorRows-paddingRows-1)
}

func (m model) modalPanelAvailableHeight() int {
	if m.height <= 0 {
		return 1
	}
	footerRows := 1
	reserved := lipgloss.Height(m.renderQueuedStatus()) + lipgloss.Height(m.renderInput()) + lipgloss.Height(m.renderInputStatus()) + footerRows
	return max(1, m.height-reserved)
}

func (m model) modalContentLines() []string {
	if m.modal == nil {
		return nil
	}
	panelWidth := min(max(48, m.width-12), 92)
	contentWidth := max(20, panelWidth-6)
	return m.modalContentLinesFor(*m.modal, contentWidth)
}

func (m model) modalContentLinesFor(modal modalState, contentWidth int) []string {
	if len(modal.Fields) == 0 {
		return nil
	}
	var lines []string
	for i, field := range modal.Fields {
		label := mutedSt.Render(field.Label + ":")
		value := field.Value
		if strings.Contains(value, "\n") {
			lines = append(lines, label)
			lines = append(lines, strings.Split(wrap(value, contentWidth), "\n")...)
		} else {
			lines = append(lines, fmt.Sprintf("%s %s", label, oneLine(value, max(8, contentWidth-lipgloss.Width(field.Label)-2))))
		}
		if i < len(modal.Fields)-1 {
			lines = append(lines, "")
		}
	}
	return lines
}
