package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m model) updateRuntimeError(err error) model {
	m.clearFakeStream()
	m.busy = false
	m.ready = false
	m.err = err.Error()
	m.status = "runtime error"
	detail := err.Error()
	if m.runtime != nil && m.runtime.logPath != "" {
		detail += "\n\nRuntime log: " + m.runtime.logPath
	}
	m.messages = append(m.messages, chatMessage{Role: "error", Content: detail})
	m.refreshViewport()
	return m
}

func (m model) updateSendDone(err error) model {
	if err == nil {
		return m
	}
	m.clearFakeStream()
	m.busy = false
	m.err = err.Error()
	m.status = "send failed"
	m.messages = append(m.messages, chatMessage{Role: "error", Content: err.Error()})
	m.refreshViewport()
	return m
}

func (m model) updateTerminalSetupResult(result terminalSetupResult) model {
	m.busy = false
	m.status = "ready"
	if result.Err != nil {
		m.err = result.Err.Error()
		m.status = "terminal setup failed"
	} else {
		m.err = ""
	}
	m.appendSystem(terminalSetupResultMessage(result))
	return m
}

func (m *model) layout() {
	chatW := max(20, m.width-4)
	inputW := max(10, m.width-6)
	promptW := lipgloss.Width(m.inputPrompt())
	m.composer.SetWidth(max(1, inputW-promptW))
	m.syncTodoOverlayState()

	input := m.renderInput()
	status := m.renderInputStatus()
	footer := m.renderFooter()
	header := m.renderHeader()
	preOverlayBottomHeight := 1 + lipgloss.Height(input) + lipgloss.Height(status) + lipgloss.Height(footer)
	bottomOverlay := m.renderBottomOverlay(max(0, min(maxBottomOverlayRows(*m), m.height-preOverlayBottomHeight-4)))
	bottomHeight := preOverlayBottomHeight + lipgloss.Height(bottomOverlay)
	mainHeight := max(1, m.height-bottomHeight)
	bodyHeight := max(1, mainHeight-lipgloss.Height(header))
	m.chatTopY = lipgloss.Height(header)
	previousYOffset := m.viewport.YOffset()
	previousWidth := m.viewport.Width()
	m.viewport.SetWidth(chatW)
	m.viewport.SetHeight(max(1, bodyHeight))
	if previousWidth != chatW {
		m.refreshViewport()
		return
	}
	m.restoreViewportPosition(previousYOffset)
}

func (m *model) appendSystem(content string) {
	m.appendLocalStatus(content)
}

func (m *model) appendSystemDirect(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	m.messages = append(m.messages, localStatusMessage(content))
	m.refreshViewport()
}

func (m *model) applyPlanView(ev runtimeEvent) {
	name := fallback(ev.Name, "plan")
	m.messages = append(m.messages, chatMessage{Role: "view", ViewType: "plan", Title: "Plan: " + name, Path: ev.Path, Content: ev.Content})
	m.refreshViewport()
}

func (m *model) applyGeneratedImage(ev runtimeEvent) {
	content := "Generated image saved."
	if ev.Path != "" {
		content += "\n\nPath: `" + ev.Path + "`"
	}
	if ev.URL != "" {
		content += "\n\nSource URL: " + ev.URL
	}
	if ev.MimeType != "" {
		content += "\n\nMIME type: `" + ev.MimeType + "`"
	}
	if ev.SizeBytes > 0 {
		content += fmt.Sprintf("\n\nSize: %d bytes", ev.SizeBytes)
	}
	m.messages = append(m.messages, chatMessage{Role: "view", ViewType: "image", Title: "Generated image", Path: ev.Path, URL: ev.URL, MimeType: ev.MimeType, SizeBytes: ev.SizeBytes, Content: content})
	m.refreshViewport()
}
