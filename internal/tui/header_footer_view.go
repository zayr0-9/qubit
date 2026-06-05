package tui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m model) renderHeader() string {
	provider := fallback(m.provider, "...")
	if m.activeKeyAlias != "" && m.activeKeyAlias != "stub" {
		provider = provider + "/" + short(strings.TrimPrefix(m.activeKeyAlias, "env:"), 14)
	}
	modelName := fallback(m.model, "...")
	sessionTitle := fallback(m.title, m.currentSessionTitle())

	appName := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("qubit")
	meta := mutedSt.Render(fmt.Sprintf("%s · %s", provider, modelName))
	headerLeft := appName
	headerRight := oneLine(sessionTitle, max(12, m.width-lipgloss.Width(headerLeft)-lipgloss.Width(meta)-8))
	headerText := fmt.Sprintf("%s  %s  %s", headerLeft, mutedSt.Render(headerRight), meta)
	return headerStyle.Width(m.width).Render(headerText)
}
func (m model) renderInputStatus() string {
	mode := m.statusModeBadges()
	if m.messageEdit.Active {
		return footerStyle.Width(m.width).Render(mode + mutedSt.Render(" · ") + messageEditInputSt.Render("editing message") + mutedSt.Render(" · enter forks/rerolls from here"))
	}
	if m.forkSelector.Active {
		if m.forkSelector.Cursor >= 0 {
			return footerStyle.Width(m.width).Render(mode + mutedSt.Render(" · ") + forkSelectInputSt.Render("selected past message") + mutedSt.Render(" · enter edit/reroll · up/down choose"))
		}
		return footerStyle.Width(m.width).Render(mode + mutedSt.Render(" · ") + forkSelectInputSt.Render("fork here") + mutedSt.Render(" · enter forks here, up chooses a previous user message"))
	}
	if m.inputHistoryActive {
		return footerStyle.Width(m.width).Render(mode + mutedSt.Render(" · ") + inputHistorySt.Render("history") + mutedSt.Render(" · up/down browse · type edits draft"))
	}

	parts := []string{m.reasoningLevelValue()}
	if cwdStatus := m.cwdStatusText(); cwdStatus != "" {
		parts = append(parts, cwdStatus)
	}
	if contextStatus := m.contextStatusText(); contextStatus != "" {
		parts = append(parts, contextStatus)
	}
	return footerStyle.Width(m.width).Render(mode + mutedSt.Render(" · "+strings.Join(parts, " · ")))
}
func (m model) cwdStatusText() string {
	if m.runtime != nil && m.runtime.launchCwd != "" {
		return m.runtime.launchCwd
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}
func (m model) statusModeBadges() string {
	if m.cwdBlockEnabled {
		return m.permissionModeBadge()
	}
	return m.permissionModeBadge() + mutedSt.Render(" · ") + m.cwdBlockBadge()
}
func (m model) permissionModeBadge() string {
	mode := m.permissionModeLabel()
	style := lipgloss.NewStyle().Bold(true)
	if m.permissionMode == permissionModeAlwaysAllow || m.permissionMode == permissionModeAllowAll {
		style = style.Foreground(green)
	} else {
		style = style.Foreground(accent)
	}
	return style.Render(mode)
}
func (m model) cwdBlockBadge() string {
	return lipgloss.NewStyle().Bold(true).Foreground(red).Render("cwd open")
}
func (m model) renderFooter() string {
	footer := "enter send | drag select transcript | ctrl+click open link if forwarded | ctrl+c copy/quit | esc clear"
	if m.keyboardEnhanced {
		footer = "enter send | shift+enter newline | shift+arrows select | ctrl+shift+left/right words | ctrl+a all | ctrl+c copy/quit"
	}
	if m.composer.HasSelection() {
		footer = "selection | ctrl+c copy | ctrl+x cut | type replace | backspace/delete remove | esc clear"
	} else if m.transcriptSelection.Active {
		footer = "transcript selection | ctrl+c copy | esc clear | wheel extends"
	}
	if m.hasPlanClarification() {
		return footerStyle.Width(m.width).Render(mutedSt.Render("up/down choose | enter answer/next | type when manual selected | esc cancel"))
	}
	if m.messageEdit.Active {
		footer = "enter fork/reroll | ctrl+j newline | esc cancel edit"
	} else if m.forkSelector.Active {
		footer = "up/down choose message | enter edit/reroll | esc cancel"
	} else if m.mode == modeModal {
		if m.modal != nil && len(m.modal.Options) > 0 {
			footer = "up/down choose option | left/right choose action | enter confirm | esc cancel"
		} else {
			footer = "left/right choose action | enter confirm | esc deny/cancel"
		}
	} else if m.mode == modeForkTree {
		if m.previousMode == modeSessionPicker {
			footer = "up/down select | pgup/pgdn session | left parent | right child | wheel preview | enter open session | esc sessions | text only"
		} else {
			footer = "up/down select | left parent | right child | wheel/pgup/pgdn preview | enter open session | esc close | text only"
		}
	} else if m.mode == modeMdEditor {
		switch m.mdEditor.View {
		case mdEditorPreview:
			footer = "ctrl+e edit | ctrl+c copy | wheel scroll | esc close"
		case mdEditorEdit:
			footer = "enter newline | ctrl+e preview | ctrl+s save | ctrl+r rename | esc close"
		case mdEditorRename:
			footer = "enter rename | esc cancel"
		case mdEditorDiscardConfirm:
			footer = "left/right choose | enter confirm | esc cancel"
		default:
			footer = "up/down select | enter open | n new doc | esc close"
		}
	} else if m.mode == modeKeyEntry {
		footer = "enter next/save | ctrl+v paste | esc cancel | secret input is masked"
	} else if m.mode == modeThemeEntry {
		footer = "up/down preset | enter apply/next | tab field | d default | esc cancel"
	} else if m.mode == modeKeyPicker {
		footer = "up/down choose key | enter activate | a add | d delete | esc close"
	} else if m.mode == modeSessionPicker {
		if m.sessionSearchMode {
			footer = "type search · up/down select · enter activate · esc clear search"
		} else {
			footer = "up/down choose session | s search | enter activate | t tree | d delete | esc close"
		}
	} else if m.mode == modeSessionPicker {
		footer = "up/down choose session | enter switch | t tree | d delete | esc close"
	} else if m.showSlashPalette() {
		footer = "up/down choose command | enter/tab complete"
	} else if m.showFileMentionPalette() {
		footer = "up/down choose file | enter/tab insert"
	}

	footerText := mutedSt.Render(footer)
	if m.err != "" {
		copyHint := ""
		if m.runtime != nil && m.runtime.logPath != "" {
			copyHint = " | log: " + m.runtime.logPath
		}
		footerText = errSt.Render(oneLine(m.err, max(20, m.width-20-len(copyHint))) + copyHint)
	}
	return footerStyle.Width(m.width).Render(footerText)
}
