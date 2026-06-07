package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return newAppView("loading...", m.terminalWindowTitle())
	}

	queuedStatus := m.renderQueuedStatus()
	input := m.renderInput()
	status := m.renderInputStatus()
	footer := m.renderFooter()
	preOverlayBottomHeight := lipgloss.Height(queuedStatus) + lipgloss.Height(input) + lipgloss.Height(status) + lipgloss.Height(footer)
	bottomOverlay := m.renderBottomOverlay(max(0, min(maxBottomOverlayRows(m), m.height-preOverlayBottomHeight-4)))
	bottomHeight := preOverlayBottomHeight + lipgloss.Height(bottomOverlay)
	mainHeight := max(0, m.height-bottomHeight)
	content := appStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			renderFixedHeight(m.renderMainArea(mainHeight), mainHeight),
			queuedStatus,
			bottomOverlay,
			input,
			status,
			footer,
		),
	)

	return newAppView(content, m.terminalWindowTitle())
}

func newAppView(content, windowTitle string) tea.View {
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.KeyboardEnhancements.ReportEventTypes = true
	view.WindowTitle = windowTitle
	return view
}

func (m model) renderMainArea(height int) string {
	if height <= 0 {
		return ""
	}
	header := m.renderHeader()
	bodyHeight := max(0, height-lipgloss.Height(header))
	chatContent := m.renderChatListView()
	if m.mode == modeSessionPicker {
		chatContent = m.renderSessionPicker(bodyHeight)
	} else if m.mode == modeKeyPicker {
		chatContent = m.renderKeyPicker()
	} else if m.mode == modeKeyEntry {
		chatContent = m.renderKeyEntry(bodyHeight)
	} else if m.mode == modeThemeEntry {
		chatContent = m.renderThemeEntry(bodyHeight)
	} else if m.mode == modeModal {
		chatContent = m.renderModal(bodyHeight)
	} else if m.mode == modeForkTree {
		chatContent = m.renderForkTreeModal(bodyHeight)
	} else if m.mode == modeMdEditor {
		chatContent = m.renderMdEditor(bodyHeight)
	} else if m.mode == modeMcpManager {
		chatContent = m.renderMcpManager(bodyHeight)
	} else if m.mode == modeMcpAddEntry {
		chatContent = m.renderMcpAddEntry(bodyHeight)
	} else if m.mode == modeMcpSecretEntry {
		chatContent = m.renderMcpSecretEntry(bodyHeight)
	}
	chat := renderChat(chatContent, m.width, max(1, bodyHeight))
	if m.mode != modeChat {
		return lipgloss.JoinVertical(lipgloss.Left, header, chat)
	}
	if m.showSlashPalette() {
		slashHeight := m.slashCommandModalHeight(bodyHeight)
		chatHeight := max(0, bodyHeight-slashHeight)
		if chatHeight > 0 {
			chat = renderChat(chatContent, m.width, chatHeight)
		} else {
			chat = ""
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, chat, m.renderSlashCommandModal(slashHeight))
	}
	if m.showFileMentionPalette() {
		fileHeight := m.fileMentionModalHeight(bodyHeight)
		chatHeight := max(0, bodyHeight-fileHeight)
		if chatHeight > 0 {
			chat = renderChat(chatContent, m.width, chatHeight)
		} else {
			chat = ""
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, chat, m.renderFileMentionModal(fileHeight))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, chat)
}

func maxBottomOverlayRows(m model) int {
	if m.hasPlanClarification() {
		return maxPlanClarificationOverlayRows
	}
	return maxTodoOverlayRows + 2
}

func (m model) renderBottomOverlay(maxHeight int) string {
	if m.hasPlanClarification() {
		return m.renderPlanClarificationOverlay(maxHeight)
	}
	return m.renderTodoOverlay(maxHeight)
}
