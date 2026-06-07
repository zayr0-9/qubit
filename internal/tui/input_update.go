package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (m model) inputSpinnerActive() bool {
	return m.busy || m.streaming || m.activeRunID != ""
}

func (m model) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.hasPlanClarification() {
		return m.updatePlanClarificationKey(msg)
	}
	if m.mode == modeModal {
		return m.updateModal(msg)
	}
	if m.mode == modeForkTree {
		return m.updateForkTree(msg)
	}
	if m.mode == modeMdEditor {
		return m.updateMdEditor(msg)
	}
	if m.mode == modeKeyEntry {
		return m.updateKeyEntry(msg)
	}
	if m.mode == modeThemeEntry {
		return m.updateThemeEntry(msg)
	}
	if m.mode == modeKeyPicker {
		return m.updateKeyPicker(msg)
	}
	if m.mode == modeSessionPicker {
		return m.updateSessionPicker(msg)
	}
	if m.forkSelector.Active {
		return m.updateForkSelector(msg)
	}
	if isOpenForkTreeShortcut(msg) {
		m.composer.Reset()
		m.layout()
		return m.openForkTree()
	}
	if isNewlineKey(msg) {
		return m.insertInputNewline()
	}

	switch msg.String() {
	case "ctrl+c":
		if m.composer.HasSelection() {
			m.status = "copied input"
			return m, copyClipboardCmd(m.composer.SelectedText())
		}
		if text := m.transcriptSelectedText(); text != "" {
			m.status = "copied transcript"
			return m, copyClipboardCmd(text)
		}
		return m, tea.Quit
	case "ctrl+x":
		if m.composer.HasSelection() {
			selected := m.composer.CutSelection()
			m.status = "cut input"
			m.layout()
			return m, copyClipboardCmd(selected)
		}
	case "esc":
		if m.showFileMentionPalette() {
			m.fileMention.Cursor = 0
			return m, nil
		}
		if m.messageEdit.Active {
			m.messageEdit = messageEditState{}
			m.composer.Reset()
			m.status = "ready"
			m.layout()
			return m, nil
		}
		if m.composer.HasSelection() {
			m.composer.ClearSelection()
			m.layout()
			return m, nil
		}
		if m.transcriptSelection.Active {
			m.transcriptSelection = transcriptSelectionState{}
			m.status = "ready"
			m.repaintTranscriptSelection()
			return m, nil
		}
		if m.streaming || (m.busy && m.activeRunID != "") {
			runID := m.activeRunID
			m.abortActiveRun()
			if runID != "" {
				return m, sendRuntime(m.runtime, map[string]any{"type": "chat.cancel", "runId": runID})
			}
			return m, nil
		}
		m.status = "ready"
		return m, nil
	case "up", "ctrl+p":
		if m.showSlashPalette() {
			m.moveSlashCursor(-1)
			return m, nil
		}
		if m.showFileMentionPalette() {
			m.moveFileMentionCursor(-1)
			return m, nil
		}
		if next, ok := m.cycleInputHistory(-1); ok {
			return next, nil
		}
	case "left":
		if m.showSlashPalette() {
			m.moveSlashCursor(-5)
			return m, nil
		}
		if m.showFileMentionPalette() {
			m.moveFileMentionCursor(-5)
			return m, nil
		}
	case "down", "ctrl+n":
		if m.showSlashPalette() {
			m.moveSlashCursor(1)
			return m, nil
		}
		if m.showFileMentionPalette() {
			m.moveFileMentionCursor(1)
			return m, nil
		}
		if next, ok := m.cycleInputHistory(1); ok {
			return next, nil
		}
	case "right":
		if m.showSlashPalette() {
			m.moveSlashCursor(5)
			return m, nil
		}
		if m.showFileMentionPalette() {
			m.moveFileMentionCursor(5)
			return m, nil
		}
	case "shift+tab":
		if m.showSlashPalette() {
			m.moveSlashCursor(-1)
			return m, nil
		}
		if m.showFileMentionPalette() {
			m.moveFileMentionCursor(-1)
			return m, nil
		}
		return m.cyclePermissionMode()
	case "tab":
		if m.showSlashPalette() {
			return m.acceptSlashSelection()
		}
		if m.showFileMentionPalette() {
			if next, ok := m.acceptFileMentionSelection(); ok {
				next.layout()
				return next, nil
			}
		}
	case "enter":
		if m.showSlashPalette() {
			return m.acceptSlashSelection()
		}
		if m.showFileMentionPalette() {
			if next, ok := m.acceptFileMentionSelection(); ok {
				next.layout()
				return next, nil
			}
		}
		return m.submitInput()
	case "pgup":
		m.chatPageUp()
		return m, nil
	case "pgdown":
		m.chatPageDown()
		return m, nil
	}

	return m.updateComposerKey(msg)
}

func (m model) updateComposerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	handled, cmd := m.composer.UpdateKey(msg)
	if handled {
		m.afterComposerEdit()
		return m, cmd
	}
	return m, nil
}

func (m model) updateTeaPaste(msg tea.PasteMsg) model {
	if m.hasPlanClarification() {
		return m.updatePlanClarificationPaste(msg.Content)
	}
	if m.mode == modeMdEditor {
		return m.updateMdEditorTeaPaste(msg)
	}
	if m.mode == modeThemeEntry {
		return m.updateThemeEntryTeaPaste(msg)
	}
	if m.mode == modeKeyEntry {
		return m.updateKeyEntryTeaPaste(msg)
	}
	if m.mode != modeChat {
		return m
	}
	return m.insertComposerPaste(msg.Content)
}

func (m model) updateComposerPaste(msg composerPasteMsg) model {
	if msg.Err != nil {
		return m.updateUIError(fmt.Errorf("paste clipboard: %w", msg.Err))
	}
	if m.hasPlanClarification() {
		return m.updatePlanClarificationPaste(msg.Text)
	}
	if m.mode == modeMdEditor {
		return m.updateMdEditorPaste(msg)
	}
	if m.mode == modeThemeEntry {
		return m.updateThemeEntryPaste(msg)
	}
	if m.mode == modeKeyEntry {
		return m.updateKeyEntryPaste(msg)
	}
	if m.mode != modeChat {
		return m
	}
	return m.insertComposerPaste(msg.Text)
}

func (m model) insertComposerPaste(text string) model {
	if text == "" {
		return m
	}
	m.composer.InsertString(text)
	m.afterComposerEdit()
	return m
}

func (m *model) afterComposerEdit() {
	m.inputHistoryActive = false
	m.inputHistoryIndex = len(m.inputHistory)
	if m.showFileMentionPalette() {
		m.ensureFileMentionIndex()
		matches := m.filteredFileMentionEntries()
		if len(matches) == 0 {
			m.fileMention.Cursor = 0
		} else if m.fileMention.Cursor >= len(matches) {
			m.fileMention.Cursor = 0
		}
	}
	m.layout()
}

func (m model) insertInputNewline() (tea.Model, tea.Cmd) {
	m.composer.InsertNewline()
	m.layout()
	return m, nil
}

func (m model) updateInputAndViewport(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func isOpenForkTreeShortcut(msg tea.KeyPressMsg) bool {
	keyEvent := msg.Key()
	return keyEvent.Code == tea.KeySpace && keyEvent.Mod&tea.ModCtrl != 0
}

func isNewlineKey(msg tea.KeyPressMsg) bool {
	if key.Matches(msg, inputNewlineBinding) {
		return true
	}
	keyEvent := msg.Key()
	if keyEvent.Code != tea.KeyEnter {
		return false
	}
	return keyEvent.Mod&tea.ModShift != 0 || keyEvent.Mod&tea.ModAlt != 0
}

func (m model) submitInput() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(normalizeInputNewlines(m.composer.Value()))
	if input == "" || !m.ready {
		return m, nil
	}
	input = m.expandFileMentionsForSend(input)

	m.composer.Reset()
	m.layout()
	m.err = ""
	if strings.HasPrefix(input, "/") {
		return m.handleSlashCommand(input)
	}
	if m.messageEdit.Active {
		if m.busy || m.streaming || m.activeRunID != "" {
			m.appendLocalStatus("Cannot submit an edited message while a run is active.")
			return m, nil
		}
		return m.submitMessageEdit(input)
	}
	if m.busy || m.streaming || m.activeRunID != "" {
		m.queueUserMessage(input)
		m.layout()
		return m, nil
	}
	m.recordInputHistory(input)
	m.saveInputHistory()
	return m.startChatRun(input)
}

func (m model) startChatRun(input string) (model, tea.Cmd) {
	runID := newRunID()
	m.messages = append(m.messages, chatMessage{Role: "user", Content: input})
	m.busy = true
	m.activeRunID = runID
	m.status = "thinking"
	m.autoScroll = true
	m.refreshViewport()
	payload := map[string]any{"type": "chat", "input": input, "runId": runID, "systemPromptMode": m.systemPromptMode(), "reasoningLevel": m.reasoningLevelValue(), "cwdBlockEnabled": m.cwdBlockEnabled}
	if m.autoNewSessionOnChat {
		payload["newSession"] = true
		payload["title"] = titleFromInput(input)
		m.autoNewSessionOnChat = false
	} else {
		payload["sessionId"] = m.session
		m.touchLocalSessionActivity(m.session, titleFromInput(input))
	}
	return m, tea.Batch(sendRuntime(m.runtime, payload), m.spinner.Tick)
}

func (m model) submitMessageEdit(input string) (tea.Model, tea.Cmd) {
	target := clampInt(m.messageEdit.MessageIndex, 0, len(m.messages))
	runID := newRunID()

	nextMessages := append([]chatMessage(nil), m.messages[:target]...)
	nextMessages = append(nextMessages, chatMessage{Role: "user", Content: input})
	m.messages = nextMessages
	m.messageEdit = messageEditState{}
	m.busy = true
	m.activeRunID = runID
	m.status = "thinking"
	m.autoScroll = true
	m.refreshViewport()

	payload := map[string]any{"type": "chat", "input": input, "runId": runID, "sessionId": m.session, "replaceFromMessageIndex": target, "title": "Edit: " + fallback(m.title, m.currentSessionTitle()), "systemPromptMode": m.systemPromptMode(), "reasoningLevel": m.reasoningLevelValue(), "cwdBlockEnabled": m.cwdBlockEnabled}
	return m, tea.Batch(sendRuntime(m.runtime, payload), m.spinner.Tick)
}
