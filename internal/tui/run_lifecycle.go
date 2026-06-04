package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

func (m *model) applyCodexUsage(usage *codexUsage) {
	if usage == nil {
		return
	}
	m.lastCodexUsage = usage
}

func (m *model) applyCodexEvent(ev runtimeEvent) {
	m.clearFakeStream()
	m.busy = false
	m.err = ""
	m.status = fallback(ev.Status, "ready")
	if ev.Provider != "" {
		m.provider = ev.Provider
	}
	if ev.ActiveProvider != "" {
		m.activeProvider = ev.ActiveProvider
	}
	if ev.ActiveKeyAlias != "" {
		m.activeKeyAlias = ev.ActiveKeyAlias
	}
	if ev.Model != "" {
		m.model = ev.Model
	}
	switch ev.Type {
	case "codex.login.started":
		copyStatus := ""
		if ev.AuthURL != "" {
			if err := clipboard.WriteAll(ev.AuthURL); err == nil {
				copyStatus = "\n\nThe URL has also been copied to your clipboard. Paste it into your browser if your terminal does not support Ctrl+Click."
			} else {
				copyStatus = "\n\nCould not copy the URL to clipboard automatically; select/copy the full URL below and paste it into your browser."
			}
		}
		m.appendSystem(fmt.Sprintf("Open this URL to sign in to ChatGPT Codex:\n%s%s", ev.AuthURL, copyStatus))
	case "codex.error":
		m.err = ev.Error
		m.status = "Codex error"
		m.messages = append(m.messages, chatMessage{Role: "error", Content: fallback(ev.Error, "Codex operation failed")})
		m.refreshViewport()
	default:
		detail := fallback(ev.Status, ev.Type)
		if ev.AccountEmail != "" {
			detail += "\nAccount: " + ev.AccountEmail
		}
		if ev.Storage != "" {
			detail += "\nStorage: " + ev.Storage
		}
		m.appendSystem(detail)
	}
}

func (m *model) applyReasoningDeltaEvent(ev runtimeEvent) {
	if ev.RunID != "" && m.activeRunID != "" && ev.RunID != m.activeRunID {
		return
	}
	delta := ev.Content
	if delta == "" {
		return
	}
	if ev.RunID != "" {
		m.activeReasoningRunID = ev.RunID
	}
	if m.activeReasoningStart < 0 {
		m.activeReasoningStart = len(m.messages)
	}
	if ev.SessionID != "" {
		m.session = ev.SessionID
	}
	if m.activeReasoningIndex < 0 || m.activeReasoningIndex != len(m.messages)-1 || m.activeReasoningIndex >= len(m.messages) || m.messages[m.activeReasoningIndex].Role != "reasoning" {
		m.messages = append(m.messages, chatMessage{Role: "reasoning", Content: delta})
		m.activeReasoningIndex = len(m.messages) - 1
	} else {
		m.messages[m.activeReasoningIndex].Content += delta
	}
	m.status = "thinking"
	m.refreshViewport()
}

func (m *model) applyAssistantEvent(ev runtimeEvent) {
	m.clearFakeStream()
	if ev.RunID != "" {
		m.activeRunID = ev.RunID
	}
	content := strings.TrimSpace(ev.Content)
	if content == "" {
		content = "(empty response)"
	}
	m.applyFinalReasoningContent(ev)
	m.applyCodexUsage(ev.CodexUsage)
	m.messages = append(m.messages, chatMessage{Role: "assistant", Content: "", ReasoningContent: ev.ReasoningContent, CodexUsage: ev.CodexUsage})
	m.activeReasoningRunID = ""
	m.activeReasoningIndex = -1
	m.activeReasoningStart = -1
	m.streaming = true
	m.streamingMessageIndex = len(m.messages) - 1
	m.streamingFullContent = content
	m.streamingVisibleRunes = 0
	m.streamingFinished = false
	m.streamingFinishStatus = ""
	if ev.SessionID != "" {
		m.session = ev.SessionID
	}
	if ev.Status != "" {
		m.status = ev.Status
	} else {
		m.status = "responding"
	}
	m.refreshViewport()
}

func (m *model) applyFinalReasoningContent(ev runtimeEvent) {
	reasoningContent := strings.TrimSpace(ev.ReasoningContent)
	if reasoningContent == "" {
		return
	}
	if ev.RunID != "" && m.activeReasoningRunID != "" && ev.RunID != m.activeReasoningRunID {
		m.messages = append(m.messages, chatMessage{Role: "reasoning", Content: reasoningContent})
		return
	}
	if m.streamedReasoningContent(m.activeReasoningStart) == reasoningContent {
		return
	}
	if m.activeReasoningIndex >= 0 && m.activeReasoningIndex < len(m.messages) && m.messages[m.activeReasoningIndex].Role == "reasoning" {
		current := strings.TrimSpace(m.messages[m.activeReasoningIndex].Content)
		switch {
		case current == reasoningContent:
			return
		case strings.HasPrefix(reasoningContent, current):
			m.messages[m.activeReasoningIndex].Content = reasoningContent
			return
		case strings.Contains(reasoningContent, current):
			m.messages[m.activeReasoningIndex].Content = reasoningContent
			return
		}
	}
	m.messages = append(m.messages, chatMessage{Role: "reasoning", Content: reasoningContent})
}

func (m *model) streamedReasoningContent(start int) string {
	parts := []string{}
	if start < 0 || start > len(m.messages) {
		start = 0
	}
	for _, message := range m.messages[start:] {
		if message.Role != "reasoning" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, "\n\n")
}

func (m model) updateFakeStreamTick() (tea.Model, tea.Cmd) {
	if !m.streaming {
		return m, nil
	}
	if m.streamingMessageIndex < 0 || m.streamingMessageIndex >= len(m.messages) {
		m.clearFakeStream()
		m.busy = false
		m.status = "ready"
		m.lastRunStartedSession = ""
		m.activeRunID = ""
		return m, nil
	}

	fullContent := []rune(m.streamingFullContent)
	m.streamingVisibleRunes = min(len(fullContent), m.streamingVisibleRunes+fakeStreamChunkSize(len(fullContent), m.streamingVisibleRunes))
	m.messages[m.streamingMessageIndex].Content = string(fullContent[:m.streamingVisibleRunes])
	m.refreshViewport()

	if m.streamingVisibleRunes < len(fullContent) {
		return m, fakeStreamTick()
	}

	finishStatus := m.streamingFinishStatus
	finished := m.streamingFinished
	m.clearFakeStream()
	var notifyCmd tea.Cmd
	if finished {
		m.status = fallback(finishStatus, "ready")
		notifyCmd = m.runCompleteNotificationCmd(finishStatus)
		m, notifyCmd = m.finishIdleAndMaybeStartQueuedUser(notifyCmd)
	} else {
		m.status = "responding"
	}
	return m, notifyCmd
}

func (m model) runCompleteNotificationCmd(status string) tea.Cmd {
	if !shouldNotifyRunComplete(status) {
		return nil
	}
	return notifyRunCompleteCmd(m.notifier, fallback(m.title, m.currentSessionTitle()))
}

func (m model) updateNotificationResult(msg notificationResultMsg) model {
	if msg.err == nil {
		return m
	}
	m.err = msg.err.Error()
	m.status = "notification failed"
	return m
}

func (m *model) abortActiveRun() {
	m.clearFakeStream()
	m.busy = false
	m.status = "aborted"
	m.lastRunStartedSession = ""
	m.activeRunID = ""
	m.refreshViewport()
}

func (m *model) clearFakeStream() {
	m.streaming = false
	m.streamingMessageIndex = 0
	m.streamingFullContent = ""
	m.streamingVisibleRunes = 0
	m.streamingFinished = false
	m.streamingFinishStatus = ""
}

func newRunID() string {
	return fmt.Sprintf("run_%d", time.Now().UnixNano())
}

func fakeStreamTick() tea.Cmd {
	return tea.Tick(18*time.Millisecond, func(time.Time) tea.Msg {
		return fakeStreamTickMsg{}
	})
}

func inputCursorPulseTick() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg {
		return inputCursorPulseMsg{}
	})
}

func fakeStreamChunkSize(totalRunes int, visibleRunes int) int {
	remaining := totalRunes - visibleRunes
	if remaining <= 0 {
		return 0
	}
	size := 3
	if totalRunes > 2000 {
		size = 24
	} else if totalRunes > 800 {
		size = 12
	} else if totalRunes > 240 {
		size = 6
	}
	return min(size, remaining)
}
