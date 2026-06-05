package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m model) isModelIdle() bool {
	return !m.busy && !m.streaming && m.activeRunID == ""
}

func (m *model) queueStatus(content string) {
	m.queueLocalMessage(queuedMessageStatus, content)
}

func (m *model) queueReminder(content string) {
	m.queueLocalMessage(queuedMessageReminder, content)
}

func (m *model) queueLocalMessage(kind queuedMessageKind, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	m.queuedMessages = append(m.queuedMessages, queuedMessage{Kind: kind, Role: "status", Content: content, SendToModel: false})
}

func (m *model) queueUserMessage(content string) {
	content = strings.TrimSpace(normalizeInputNewlines(content))
	if content == "" {
		return
	}
	m.queuedMessages = append(m.queuedMessages, queuedMessage{Kind: queuedMessageUser, Role: "user", Content: content, SendToModel: true})
	m.status = "message queued"
}

func (m *model) appendLocalStatus(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	if m.isModelIdle() {
		m.messages = append(m.messages, localStatusMessage(content))
		m.refreshViewport()
		return
	}
	m.queueStatus(content)
}

func localStatusMessage(content string) chatMessage {
	return chatMessage{Role: "status", Content: content, LocalOnly: true, MessageKind: messageKindStatus}
}

func localReminderMessage(content string) chatMessage {
	return chatMessage{Role: "status", Content: content, LocalOnly: true, MessageKind: messageKindReminder}
}

func (m *model) flushDisplayQueue() bool {
	if !m.isModelIdle() || len(m.queuedMessages) == 0 {
		return false
	}
	remaining := m.queuedMessages[:0]
	flushed := false
	for _, queued := range m.queuedMessages {
		if queued.SendToModel || queued.Kind == queuedMessageUser {
			remaining = append(remaining, queued)
			continue
		}
		switch queued.Kind {
		case queuedMessageReminder:
			m.messages = append(m.messages, localReminderMessage(queued.Content))
		default:
			m.messages = append(m.messages, localStatusMessage(queued.Content))
		}
		flushed = true
	}
	m.queuedMessages = remaining
	if flushed {
		m.refreshViewport()
	}
	return flushed
}

func (m *model) popQueuedUserMessage() (string, bool) {
	for i, queued := range m.queuedMessages {
		if queued.Kind != queuedMessageUser || !queued.SendToModel {
			continue
		}
		m.queuedMessages = append(m.queuedMessages[:i], m.queuedMessages[i+1:]...)
		return queued.Content, true
	}
	return "", false
}

func (m model) hasQueuedUserMessages() bool {
	for _, queued := range m.queuedMessages {
		if queued.Kind == queuedMessageUser && queued.SendToModel {
			return true
		}
	}
	return false
}

func (m model) finishIdleAndMaybeStartQueuedUser(notifyCmd tea.Cmd) (model, tea.Cmd) {
	m.busy = false
	m.lastRunStartedSession = ""
	m.activeRunID = ""
	m.clearActiveRunStartedAt()
	m.flushDisplayQueue()
	if input, ok := m.popQueuedUserMessage(); ok {
		// A queued user message immediately starts the next run; suppress the
		// previous run's completion notification to avoid nested Batch commands
		// and noisy back-to-back notifications.
		next, cmd := m.startChatRun(input)
		return next, cmd
	}
	return m, notifyCmd
}

func (m model) renderQueuedStatus() string {
	if len(m.queuedMessages) == 0 {
		return ""
	}
	width := max(20, m.width-4)
	visible := []string{}
	userCount := 0
	for _, queued := range m.queuedMessages {
		prefix := "queued"
		if queued.Kind == queuedMessageUser {
			userCount++
			prefix = "queued send"
		} else if queued.Kind == queuedMessageReminder {
			prefix = "reminder"
		}
		line := prefix + ": " + oneLine(queued.Content, max(8, width-lipgloss.Width(prefix)-4))
		visible = append(visible, mutedSt.Render(line))
		if len(visible) >= 3 {
			break
		}
	}
	remaining := len(m.queuedMessages) - len(visible)
	if remaining > 0 {
		visible = append(visible, mutedSt.Render("+"+itoa(remaining)+" queued update(s)"))
	}
	if userCount > 0 && len(visible) < 4 {
		visible = append(visible, mutedSt.Render("queued messages send when current run finishes"))
	}
	return inputStyle.Width(m.width).Render(strings.Join(visible, "\n"))
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	digits := []byte{}
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
