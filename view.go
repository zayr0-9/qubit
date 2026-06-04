package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

func (m model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return newAppView("loading...")
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

	return newAppView(content)
}

func newAppView(content string) tea.View {
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.KeyboardEnhancements.ReportEventTypes = true
	return view
}

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

func (m model) renderMainArea(height int) string {
	if height <= 0 {
		return ""
	}
	header := m.renderHeader()
	bodyHeight := max(0, height-lipgloss.Height(header))
	chatContent := m.viewport.View()
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
	if contextStatus := m.contextStatusText(); contextStatus != "" {
		parts = append(parts, contextStatus)
	}
	return footerStyle.Width(m.width).Render(mode + mutedSt.Render(" · "+strings.Join(parts, " · ")))
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
		case mdEditorEdit:
			footer = "ctrl+s save | ctrl+r rename | esc close | raw markdown"
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

func (m model) renderSessionPicker(height int) string {
	sessions := m.sessionPickerSessions()
	panelWidth := max(20, m.width-4)
	contentWidth := max(20, panelWidth-4)
	rowWidth := max(20, contentWidth-2)
	statsWidth := m.sessionPickerStatsWidth(sessions)

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Bold(true).Render("sessions") + "\n")
	if m.sessionSearchMode {
		query := m.sessionSearchQuery
		if query == "" {
			query = mutedSt.Render("type title...")
		}
		b.WriteString(mutedSt.Render("search ") + lipgloss.NewStyle().Foreground(accent).Render(query) + "\n")
		b.WriteString(mutedSt.Render("↑/↓ select · enter activate · esc clear search") + "\n\n")
	} else {
		b.WriteString(mutedSt.Render("↑/↓ select · enter activate · s search · t tree · d delete · esc close") + "\n\n")
	}
	if len(sessions) == 0 {
		if strings.TrimSpace(m.sessionSearchQuery) != "" {
			b.WriteString(mutedSt.Render("no matching sessions"))
		} else {
			b.WriteString(mutedSt.Render("no sessions yet · esc then /new to create one"))
		}
		return lipgloss.NewStyle().Padding(1, 2).Width(panelWidth).Render(b.String())
	}

	maxRows := max(1, height-6)
	if !m.sessionSearchMode {
		maxRows = max(1, height-5)
	}
	window := visibleListWindow(len(sessions), m.sessionCursor, maxRows)
	if window.HasAbove {
		b.WriteString(mutedSt.Render(fmt.Sprintf("  more above (%d)", window.Start)))
		b.WriteString("\n")
	}
	for i := window.Start; i < window.End; i++ {
		session := sessions[i]
		line := renderSessionPickerRow(session, session.ID == m.session, rowWidth, statsWidth, m.sessionForkCount(session.ID))
		if i == m.sessionCursor {
			line = selectSt.Render("  " + line)
		} else {
			line = mutedSt.Render("  ") + line
		}
		b.WriteString(line)
		if i < window.End-1 || window.HasBelow {
			b.WriteString("\n")
		}
	}
	if window.HasBelow {
		b.WriteString(mutedSt.Render(fmt.Sprintf("  more below (%d)", len(sessions)-window.End)))
	}
	return lipgloss.NewStyle().Padding(1, 2).Width(panelWidth).Render(b.String())
}

func (m model) sessionPickerStatsWidth(sessions []sessionInfo) int {
	statsWidth := lipgloss.Width(formatSessionPickerStats(sessionInfo{}, 0))
	for _, session := range sessions {
		statsWidth = max(statsWidth, lipgloss.Width(formatSessionPickerStats(session, m.sessionForkCount(session.ID))))
	}
	return statsWidth
}

func (m model) sessionForkCount(sessionID string) int {
	if sessionID == "" {
		return 0
	}
	count := 0
	for _, session := range m.sessions {
		if session.ForkedFromSessionID == "" || session.ID == sessionID {
			continue
		}
		seen := map[string]bool{session.ID: true}
		for parentID := session.ForkedFromSessionID; parentID != ""; parentID = m.parentSessionID(parentID) {
			if parentID == sessionID {
				count++
				break
			}
			if seen[parentID] {
				break
			}
			seen[parentID] = true
		}
	}
	return count
}

func (m model) parentSessionID(sessionID string) string {
	for _, session := range m.sessions {
		if session.ID == sessionID {
			return session.ForkedFromSessionID
		}
	}
	return ""
}

func renderSessionPickerRow(session sessionInfo, active bool, width int, statsWidth int, forkCount int) string {
	activeMarker := " "
	if active {
		activeMarker = "•"
	}
	favouriteMarker := " "
	if strings.TrimSpace(session.FavouritedAt) != "" {
		favouriteMarker = "★"
	}
	prefix := activeMarker + " " + favouriteMarker
	stats := formatSessionPickerStats(session, forkCount)
	statsWidth = max(statsWidth, lipgloss.Width(stats))
	titleWidth := max(1, width-lipgloss.Width(prefix)-statsWidth-3)
	if titleWidth+lipgloss.Width(prefix)+statsWidth+3 > width {
		stats = oneLine(stats, max(1, width-lipgloss.Width(prefix)-4))
		statsWidth = lipgloss.Width(stats)
		titleWidth = max(1, width-lipgloss.Width(prefix)-statsWidth-3)
	}
	title := oneLine(session.Title, titleWidth)
	return fmt.Sprintf("%s %-*s %*s", prefix, titleWidth, title, statsWidth, stats)
}

func formatSessionPickerStats(session sessionInfo, forkCount int) string {
	parts := []string{formatSessionCreatedAt(session.CreatedAt)}
	parts = append(parts, fmt.Sprintf("%d %s", session.MessageCount, pluralLabel(session.MessageCount, "msg", "msgs")))
	parts = append(parts, fmt.Sprintf("%d %s", forkCount, pluralLabel(forkCount, "fork", "forks")))
	return strings.Join(parts, " · ")
}

func formatSessionCreatedAt(createdAt string) string {
	createdAt = strings.TrimSpace(createdAt)
	if createdAt == "" {
		return "-- -- --:--"
	}
	if t, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
		return t.Local().Format("02-01 15:04")
	}
	if t, err := time.Parse("2006-01-02T15:04:05", createdAt); err == nil {
		return t.Local().Format("02-01 15:04")
	}
	return oneLine(createdAt, len("02-01 15:04"))
}

func pluralLabel(count int, singular string, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
func (m *model) refreshViewport() {
	previousYOffset := m.viewport.YOffset()
	m.toolHitboxes = nil
	m.linkHitboxes = nil
	m.transcriptLines = nil
	var b strings.Builder
	contentLine := 0
	for i := 0; i < len(m.messages); i++ {
		message := m.messages[i]
		if i > 0 {
			separator := messageSeparator(m.messages[i-1], message)
			b.WriteString(separator)
			contentLine += separatorBlankLineCount(separator)
		}
		if message.Role == "view" {
			rendered := m.renderViewMessage(message)
			b.WriteString(rendered)
			contentLine += renderedLineCount(rendered)
			continue
		}
		if message.Role == "tool" {
			startLine := contentLine
			groups := []*toolGroup{}
			for i < len(m.messages) && m.messages[i].Role == "tool" {
				if m.messages[i].ToolGroup != nil {
					groups = append(groups, m.messages[i].ToolGroup)
				}
				i++
			}
			i--
			rendered := m.renderCollapsedToolGroups(groups, max(20, m.viewport.Width()))
			b.WriteString(rendered)
			m.appendToolGroupHitboxes(groups, startLine, max(20, m.viewport.Width()))
			contentLine += renderedLineCount(rendered)
			continue
		}
		if message.Role == "reasoning" {
			rendered := m.renderReasoningBlock(message, max(20, m.viewport.Width()))
			b.WriteString(rendered)
			headingWidth := lipgloss.Width(m.renderReasoningBlockHeading(message))
			m.toolHitboxes = append(m.toolHitboxes, toolHitbox{Kind: "reasoning", MessageIndex: i, StartY: contentLine, EndY: contentLine, StartX: 0, EndX: max(0, headingWidth-1)})
			contentLine += renderedLineCount(rendered)
			continue
		}
		cacheable := !(m.streaming && i == m.streamingMessageIndex)
		rendered := renderMessageWithIcon(message, m.renderMessageContent(message, cacheable), messageDisplayNumber(m.messages, i))
		b.WriteString(rendered)
		contentLine += renderedLineCount(rendered)
		if message.Role == "user" && i == len(m.messages)-1 {
			b.WriteString("\n")
			contentLine++
		}
	}
	content := b.String()
	m.transcriptContent = content
	m.transcriptLines = transcriptRenderLines(content)
	m.linkHitboxes = transcriptLinkHitboxes(m.transcriptLines)
	m.repaintTranscriptSelection()
	m.restoreViewportPosition(previousYOffset)
}

func (m *model) restoreViewportPosition(yOffset int) {
	if m.autoScroll {
		m.viewport.GotoBottom()
		return
	}
	m.viewport.SetYOffset(clampInt(yOffset, 0, max(0, m.viewport.TotalLineCount()-m.viewport.Height())))
}

func messageSeparator(prev chatMessage, next chatMessage) string {
	if prev.Role == "user" {
		return "\n\n"
	}
	if prev.Role == "tool" || next.Role == "tool" {
		return "\n"
	}
	if prev.Role == next.Role {
		return "\n"
	}
	return "\n\n"
}
func separatorBlankLineCount(separator string) int {
	newlines := strings.Count(separator, "\n")
	if newlines == 0 {
		return 0
	}
	return newlines - 1
}

func (m *model) renderViewMessage(message chatMessage) string {
	title := fallback(message.Title, "View")
	isPlan := message.ViewType == "plan"
	if isPlan && !strings.HasPrefix(title, "Plan:") {
		title = "Plan: " + title
	}

	width := max(20, m.viewport.Width()-2)
	if isPlan {
		return m.renderPlanViewMessage(message, title, width)
	}

	header := aiIcon.Render("◇") + " " + lipgloss.NewStyle().Foreground(accent).Bold(true).Render(title)
	if message.Path != "" {
		header += mutedSt.Render(" · " + oneLine(message.Path, max(12, m.viewport.Width()-lipgloss.Width(title)-8)))
	}
	content, err := renderMessageContentAtWidth(chatMessage{Role: "assistant", Content: message.Content}, width)
	if err != nil {
		content = wrap(message.Content, width)
	}
	if strings.TrimSpace(content) == "" {
		return header
	}
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	return header + "\n" + strings.Join(lines, "\n")
}

func (m *model) renderPlanViewMessage(message chatMessage, title string, width int) string {
	panelWidth := min(max(44, width), 96)
	contentWidth := max(20, panelWidth-6)
	header := aiIcon.Render("◇") + " " + lipgloss.NewStyle().Foreground(accent).Bold(true).Render(oneLine(title, contentWidth))
	if message.Path != "" {
		header += mutedSt.Render(" · " + oneLine(message.Path, max(12, contentWidth-lipgloss.Width(stripANSI(title))-3)))
	}
	content, err := renderMessageContentAtWidth(chatMessage{Role: "assistant", Content: message.Content}, contentWidth)
	if err != nil {
		content = wrap(message.Content, contentWidth)
	}
	body := header
	if strings.TrimSpace(content) != "" {
		body += "\n" + strings.Join(strings.Split(content, "\n"), "\n")
	}
	return renderAccentBorderedPanel(body, panelWidth)
}

func renderAccentBorderedPanel(body string, width int) string {
	return lipgloss.NewStyle().
		Foreground(text).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(0, 2).
		Width(width).
		Render(body)
}

func (m *model) renderReasoningBlock(message chatMessage, width int) string {
	heading := m.renderReasoningBlockHeading(message)
	if !message.Expanded {
		return heading
	}
	content := strings.TrimSpace(stripReasoningTitle(message.Content))
	if content == "" {
		return heading
	}
	lines := strings.Split(wrap(content, max(20, width-4)), "\n")
	for i := range lines {
		lines[i] = reasoningSt.Render("  " + lines[i])
	}
	return heading + "\n" + strings.Join(lines, "\n")
}

func (m *model) renderReasoningBlockHeading(message chatMessage) string {
	icon := "✦"
	title := reasoningBlockTitle(message.Content)
	return reasoningSt.Render(icon) + " " + reasoningSt.Bold(true).Render(title)
}

func reasoningBlockTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		title, ok := boldMarkdownLineTitle(line)
		if ok {
			return title
		}
	}
	preview := oneLine(strings.TrimSpace(stripReasoningTitle(content)), 56)
	if preview == "" {
		return "Thinking"
	}
	return preview
}

func boldMarkdownLineTitle(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "**") {
		return "", false
	}
	rest := strings.TrimPrefix(trimmed, "**")
	end := strings.Index(rest, "**")
	if end < 0 {
		return "", false
	}
	title := strings.TrimSpace(rest[:end])
	return title, title != ""
}

func stripReasoningTitle(content string) string {
	lines := strings.Split(content, "\n")
	for len(lines) > 0 {
		if _, ok := boldMarkdownLineTitle(lines[0]); ok {
			lines = lines[1:]
			for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
				lines = lines[1:]
			}
			break
		}
		if strings.TrimSpace(lines[0]) != "" {
			break
		}
		lines = lines[1:]
	}
	return strings.Join(lines, "\n")
}

func renderMessageWithIcon(message chatMessage, content string, number int) string {
	icon := aiIcon.Render("◆")
	if message.LocalOnly || message.Role == "status" {
		icon = mutedSt.Render("◇")
	} else if message.Role == "reasoning" {
		icon = mutedSt.Render("◇")
	} else if message.Role == "user" {
		if number > 0 {
			icon = userIcon.Render(fmt.Sprintf("›%d", number))
		} else {
			icon = userIcon.Render("›")
		}
	} else if message.Role == "error" {
		icon = errorIcon.Render("!")
	}

	if content == "" {
		return icon
	}

	lines := strings.Split(content, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) == 0 {
		return icon
	}
	indent := strings.Repeat(" ", max(2, lipgloss.Width(icon)+1))
	lines[0] = icon + " " + strings.TrimLeft(lines[0], " \t")
	for i := 1; i < len(lines); i++ {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}

func messageDisplayNumber(messages []chatMessage, index int) int {
	if index < 0 || index >= len(messages) || messages[index].Role != "user" {
		return 0
	}
	return index + 1
}
func (m *model) renderMessageContent(message chatMessage, cacheable bool) string {
	width := max(20, m.viewport.Width())
	key := renderCacheKey{Role: message.Role, Content: message.Content, ReasoningContent: message.ReasoningContent, Width: width}
	if cacheable && m.renderCache != nil {
		if cached, ok := m.renderCache[key]; ok {
			return cached
		}
	}

	rendered, err := renderMessageContentAtWidth(message, width)
	if err != nil {
		rendered = wrap(message.Content, width)
	}
	if cacheable && m.renderCache != nil {
		m.renderCache[key] = rendered
	}
	return rendered
}

func renderMessageContentAtWidth(message chatMessage, width int) (string, error) {
	width = max(20, width)
	if message.LocalOnly || message.Role == "status" || message.Role == "error" || message.Role == "reasoning" {
		return wrap(message.Content, width), nil
	}
	markdown, err := renderMarkdown(message.Content, width)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(stripBackgroundANSI(markdown), "\n"), nil
}

func renderMarkdown(markdown string, width int) (string, error) {
	renderWidth := max(20, width)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(noBackgroundMarkdownStyle()),
		glamour.WithWordWrap(renderWidth),
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return "", fmt.Errorf("create markdown renderer: %w", err)
	}

	rendered, err := renderer.Render(markdown)
	if err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return rendered, nil
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func uintPtr(value uint) *uint {
	return &value
}

func colorToHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

func noBackgroundMarkdownStyle() ansi.StyleConfig {
	style := styles.DarkStyleConfig
	style.Document.Margin = uintPtr(0)
	style.H1.BackgroundColor = nil
	style.H1.Color = stringPtr(colorToHex(accent))
	style.H1.Bold = boolPtr(true)
	style.H2.Color = stringPtr(colorToHex(accent))
	style.H2.Bold = boolPtr(true)
	style.H3.Color = stringPtr(colorToHex(cyan))
	style.H3.Bold = boolPtr(true)
	style.BlockQuote.Color = stringPtr(colorToHex(muted))
	style.Code.Color = stringPtr(colorToHex(cyan))
	style.Code.BackgroundColor = nil
	style.CodeBlock.Margin = uintPtr(0)
	if style.CodeBlock.Chroma != nil {
		style.CodeBlock.Chroma.Error.BackgroundColor = nil
		style.CodeBlock.Chroma.Background.BackgroundColor = nil
	}
	return style
}

func stripBackgroundANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != '\x1b' || i+1 >= len(s) || s[i+1] != '[' {
			b.WriteByte(s[i])
			i++
			continue
		}

		end := i + 2
		for end < len(s) && s[end] != 'm' {
			end++
		}
		if end >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}

		sequence := s[i+2 : end]
		kept := keepNonBackgroundSGR(sequence)
		if len(kept) > 0 {
			b.WriteString("\x1b[")
			b.WriteString(strings.Join(kept, ";"))
			b.WriteByte('m')
		}
		i = end + 1
	}
	return b.String()
}

func keepNonBackgroundSGR(sequence string) []string {
	if sequence == "" {
		return []string{"0"}
	}

	parts := strings.Split(sequence, ";")
	kept := make([]string, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		switch part {
		case "40", "41", "42", "43", "44", "45", "46", "47", "48", "49", "100", "101", "102", "103", "104", "105", "106", "107":
			if part == "48" {
				i = skipExtendedSGR(parts, i)
			}
			continue
		default:
			kept = append(kept, part)
		}
	}
	return kept
}

func skipExtendedSGR(parts []string, i int) int {
	if i+1 >= len(parts) {
		return i
	}
	switch parts[i+1] {
	case "5":
		if i+2 < len(parts) {
			return i + 2
		}
	case "2":
		if i+4 < len(parts) {
			return i + 4
		}
	}
	return i + 1
}

func renderChat(content string, width int, height int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		lines[i] = chatStyle.Render(line)
	}
	return strings.Join(lines, "\n")
}

func renderFixedHeight(content string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
