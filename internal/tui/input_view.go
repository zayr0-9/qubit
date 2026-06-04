package tui

import "charm.land/lipgloss/v2"

func (m model) renderInput() string {
	view := m.composer.ViewStyled(m.inputPrompt(), m.inputCursorPulse, m.composerTextStyle())
	return inputStyle.Width(m.width).Height(m.composer.Height()).Render(view)
}
func (m model) composerTextStyle() lipgloss.Style {
	switch {
	case m.messageEdit.Active:
		return messageEditInputSt
	case m.forkSelector.Active && m.forkSelector.Cursor >= 0:
		return forkSelectInputSt
	case m.inputHistoryActive:
		return inputHistorySt
	default:
		return lipgloss.Style{}
	}
}
func (m model) inputPrompt() string {
	if m.inputSpinnerActive() {
		return m.spinner.View() + " "
	}
	return idleInputPrompt()
}
func idleInputPrompt() string {
	return lipgloss.NewStyle().Foreground(accent).Render("› ")
}
