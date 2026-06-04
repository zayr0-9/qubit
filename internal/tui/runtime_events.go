package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

func (m model) updateRuntime(ev runtimeEvent) (tea.Model, tea.Cmd) {
	switch ev.Type {
	case "ready":
		m.ready = true
		m.provider = ev.Provider
		m.activeProvider = ev.ActiveProvider
		if m.activeProvider == "" {
			m.activeProvider = ev.Provider
		}
		m.activeKeyAlias = ev.ActiveKeyAlias
		m.model = ev.Model
		m.maxContext = ev.MaxContext
		m.reasoningLevel = normalizeReasoningLevel(ev.ReasoningLevel)
		if ev.WorkspaceCwd != "" {
			m.runtime.launchCwd = ev.WorkspaceCwd
		}
		m.session = ev.SessionID
		m.title = ""
		m.autoNewSessionOnChat = true
		m.status = "ready"
		return m, tea.Batch(waitRuntimeEvent(m.runtime), sendRuntime(m.runtime, map[string]any{"type": "session.list"}))
	case "run_started":
		if !m.acceptRunScopedEvent(ev) {
			return m, waitRuntimeEvent(m.runtime)
		}
		m.busy = true
		if ev.RunID != "" {
			m.activeRunID = ev.RunID
		}
		m.activeReasoningRunID = ev.RunID
		m.activeReasoningIndex = -1
		m.activeReasoningStart = len(m.messages)
		m.status = "thinking"
		if ev.SessionID != "" {
			m.session = ev.SessionID
			m.lastRunStartedSession = ev.SessionID
			m.touchLocalSessionActivity(ev.SessionID, m.latestUserMessageTitle())
		}
	case "reasoning.delta":
		if !m.acceptRunScopedEvent(ev) {
			return m, waitRuntimeEvent(m.runtime)
		}
		m.applyReasoningDeltaEvent(ev)
	case "assistant":
		if !m.acceptRunScopedEvent(ev) {
			return m, waitRuntimeEvent(m.runtime)
		}
		m.applyAssistantEvent(ev)
		return m, tea.Batch(waitRuntimeEvent(m.runtime), fakeStreamTick())
	case "codex.usage":
		if !m.acceptRunScopedEvent(ev) {
			return m, waitRuntimeEvent(m.runtime)
		}
		m.applyCodexUsage(ev.CodexUsage)
	case "run_finished":
		if !m.acceptRunScopedEvent(ev) {
			return m, waitRuntimeEvent(m.runtime)
		}
		if ev.SessionID != "" {
			m.session = ev.SessionID
		}
		m.applyCodexUsage(ev.CodexUsage)
		finishStatus := ev.Status
		if finishStatus == "" {
			finishStatus = "ready"
		}
		var notifyCmd tea.Cmd
		if m.streaming {
			m.streamingFinished = true
			m.streamingFinishStatus = finishStatus
			m.status = "responding"
		} else {
			m.status = finishStatus
			notifyCmd = m.runCompleteNotificationCmd(finishStatus)
			m, notifyCmd = m.finishIdleAndMaybeStartQueuedUser(notifyCmd)
		}
		return m, tea.Batch(waitRuntimeEvent(m.runtime), sendRuntime(m.runtime, map[string]any{"type": "session.list"}), notifyCmd)
	case "session.list":
		m.applySessionList(ev)
	case "session.tree":
		m.applyForkTree(ev)
	case "md.list":
		m.applyMdList(ev)
	case "md.read":
		m.applyMdRead(ev)
	case "md.created":
		m.applyMdCreated(ev)
	case "md.saved":
		m.applyMdSaved(ev)
	case "md.renamed":
		m.applyMdRenamed(ev)
	case "key.list":
		m.applyKeyList(ev)
	case "model.list":
		if ev.Model != "" {
			m.model = ev.Model
		}
		m.maxContext = ev.MaxContext
		m.reasoningLevel = normalizeReasoningLevel(ev.ReasoningLevel)
		if len(ev.Models) > 0 {
			m.models = ev.Models
		}
		m.applyActiveKeyMetadata(ev)
		m = m.openModelSelectorModal(ev.Models)
	case "model.updated":
		m.applyModelUpdated(ev)
	case "reasoning.updated":
		m.applyReasoningUpdated(ev)
	case "key.updated":
		m.applyKeyUpdated(ev)
	case "codex.login.started", "codex.login.completed", "codex.login.cancelled", "codex.logout.completed", "codex.status", "codex.error":
		m.applyCodexEvent(ev)
	case "tool.permission.request":
		if !m.acceptRunScopedEvent(ev) {
			return m, waitRuntimeEvent(m.runtime)
		}
		if m.shouldAutoAllowPermission(ev) {
			m.status = "thinking"
			return m, tea.Batch(sendRuntime(m.runtime, map[string]any{"type": "tool.permission.response", "id": ev.ID, "allow": true}), waitRuntimeEvent(m.runtime))
		}
		m = m.openToolPermissionModal(ev)
	case "tool.call.start":
		if !m.acceptRunScopedEvent(ev) {
			return m, waitRuntimeEvent(m.runtime)
		}
		m.applyToolCallStart(ev)
		return m, tea.Batch(waitRuntimeEvent(m.runtime), toolCallRevealTick())
	case "tool.call.finish":
		if !m.acceptRunScopedEvent(ev) {
			return m, waitRuntimeEvent(m.runtime)
		}
		m.applyToolCallFinish(ev)
	case "plan.view":
		if !m.acceptRunScopedEvent(ev) {
			return m, waitRuntimeEvent(m.runtime)
		}
		m.applyPlanView(ev)
	case "plan.clarification.request":
		if !m.acceptRunScopedEvent(ev) {
			return m, waitRuntimeEvent(m.runtime)
		}
		m = m.openPlanClarification(ev)
	case "generated.image":
		m.applyGeneratedImage(ev)
	case "session.created":
		m.clearFakeStream()
		m.activeRunID = ""
		m.autoScroll = true
		m.busy = false
		m.session = ev.SessionID
		m.title = ev.SessionTitle
		m.autoNewSessionOnChat = false
		m.messages = []chatMessage{{Role: "assistant", Content: fmt.Sprintf("Started new session: %s (%s)", m.title, short(m.session, 18))}}
		m.status = "ready"
		m.refreshViewport()
	case "session.activated", "session.forked":
		m.clearFakeStream()
		m.activeRunID = ""
		m.autoScroll = true
		m.busy = true
		m.session = ev.SessionID
		m.title = ev.SessionTitle
		m.autoNewSessionOnChat = false
		verb := "Loading session"
		if ev.Type == "session.forked" {
			verb = "Loading fork"
		}
		m.messages = []chatMessage{{Role: "assistant", Content: fmt.Sprintf("%s: %s (%s)", verb, m.title, short(m.session, 18))}}
		m.status = "loading transcript"
		m.refreshViewport()
		return m, tea.Batch(waitRuntimeEvent(m.runtime), sendRuntime(m.runtime, map[string]any{"type": "session.messages", "sessionId": m.session}))
	case "session.messages":
		m.applySessionMessages(ev)
	case "session.deleted":
		m.applySessionDeleted(ev)
	case "session.favourited":
		m.applySessionFavourited(ev)
	case "session.renamed":
		m.busy = false
		if ev.SessionID != "" {
			m.session = ev.SessionID
		}
		if ev.SessionTitle != "" {
			m.title = ev.SessionTitle
		}
		m.appendSystem("Renamed current session to: " + m.title)
	case "error":
		m.clearFakeStream()
		m.busy = false
		m.lastRunStartedSession = ""
		m.activeRunID = ""
		m.err = ev.Error
		m.status = "error"
		m.messages = append(m.messages, chatMessage{Role: "error", Content: ev.Error})
		m.refreshViewport()
	}
	return m, waitRuntimeEvent(m.runtime)
}

func (m model) acceptRunScopedEvent(ev runtimeEvent) bool {
	if ev.RunID == "" {
		return true
	}
	return m.activeRunID != "" && ev.RunID == m.activeRunID
}
