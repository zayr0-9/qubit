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
		m.appendSystemDirect(fmt.Sprintf("Open this URL to sign in to ChatGPT Codex:\n%s%s", ev.AuthURL, copyStatus))
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
		m.appendSystemDirect(detail)
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
	if m.reconcileFinalReasoningContent(reasoningContent) {
		return
	}
	m.messages = append(m.messages, chatMessage{Role: "reasoning", Content: reasoningContent})
}

func (m *model) reconcileFinalReasoningContent(reasoningContent string) bool {
	indexes := m.reasoningBlockIndexesSince(m.activeReasoningStart)
	if len(indexes) == 0 {
		return false
	}
	if len(indexes) == 1 {
		idx := indexes[0]
		current := strings.TrimSpace(m.messages[idx].Content)
		if current != reasoningContent {
			m.messages[idx].Content = reasoningContent
		}
		return true
	}
	streamed := m.streamedReasoningContentFromIndexes(indexes)
	if m.replaceMatchingReasoningBlocks(indexes, reasoningContent) {
		return true
	}
	if reasoningEquivalent(streamed, reasoningContent) || reasoningLooseMatch(streamed, reasoningContent) {
		m.replaceReasoningBlocksFromFinal(indexes, reasoningContent)
		return true
	}
	if m.activeReasoningIndex >= 0 && m.activeReasoningIndex < len(m.messages) && m.messages[m.activeReasoningIndex].Role == "reasoning" {
		current := strings.TrimSpace(m.messages[m.activeReasoningIndex].Content)
		switch {
		case current == reasoningContent:
			return true
		case strings.HasPrefix(reasoningContent, current), strings.Contains(reasoningContent, current):
			m.messages[m.activeReasoningIndex].Content = reasoningContent
			return true
		}
	}
	return false
}

func (m *model) reasoningBlockIndexesSince(start int) []int {
	if start < 0 || start > len(m.messages) {
		start = 0
	}
	indexes := []int{}
	for i := start; i < len(m.messages); i++ {
		if m.messages[i].Role == "reasoning" && strings.TrimSpace(m.messages[i].Content) != "" {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func (m *model) replaceReasoningBlocksFromFinal(indexes []int, reasoningContent string) {
	parts := splitReasoningSections(reasoningContent)
	if len(parts) != len(indexes) {
		return
	}
	for i, idx := range indexes {
		m.messages[idx].Content = parts[i]
	}
}

func (m *model) replaceMatchingReasoningBlocks(indexes []int, reasoningContent string) bool {
	parts := splitReasoningSections(reasoningContent)
	if len(parts) != len(indexes) {
		return false
	}
	matched := false
	for i, idx := range indexes {
		current := strings.TrimSpace(m.messages[idx].Content)
		part := strings.TrimSpace(parts[i])
		if part == "" {
			continue
		}
		if current == part {
			matched = true
			continue
		}
		if reasoningLooseMatch(current, part) || strings.Contains(part, current) || strings.Contains(current, part) || reasoningEditDistanceClose(current, part) {
			m.messages[idx].Content = part
			matched = true
		}
	}
	return matched
}

func (m *model) streamedReasoningContent(start int) string {
	return m.streamedReasoningContentFromIndexes(m.reasoningBlockIndexesSince(start))
}

func (m *model) streamedReasoningContentFromIndexes(indexes []int) string {
	parts := []string{}
	for _, idx := range indexes {
		if idx < 0 || idx >= len(m.messages) || m.messages[idx].Role != "reasoning" {
			continue
		}
		content := strings.TrimSpace(m.messages[idx].Content)
		if content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, "\n\n")
}

func splitReasoningSections(content string) []string {
	sections := strings.Split(strings.TrimSpace(content), "\n\n")
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section != "" {
			parts = append(parts, section)
		}
	}
	return parts
}

func reasoningEquivalent(a, b string) bool {
	return normalizeReasoningForCompare(a) == normalizeReasoningForCompare(b)
}

func reasoningLooseMatch(a, b string) bool {
	na := normalizeReasoningForCompare(a)
	nb := normalizeReasoningForCompare(b)
	if na == "" || nb == "" {
		return false
	}
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		return true
	}
	shorter, longer := na, nb
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	return len(shorter) >= 20 && longestCommonReasoningRunes(shorter, longer)*100/len(longer) >= 70
}

func normalizeReasoningForCompare(content string) string {
	return strings.ToLower(strings.Join(strings.Fields(content), " "))
}

func reasoningEditDistanceClose(a, b string) bool {
	na := normalizeReasoningForCompare(a)
	nb := normalizeReasoningForCompare(b)
	if len(na) < 8 || len(nb) < 8 {
		return false
	}
	shorter, longer := na, nb
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	allowed := max(2, len(longer)/5)
	return levenshteinDistanceAtMost(shorter, longer, allowed)
}

func levenshteinDistanceAtMost(a, b string, limit int) bool {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) > len(br) {
		ar, br = br, ar
	}
	if len(br)-len(ar) > limit {
		return false
	}
	prev := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		current := make([]int, len(br)+1)
		current[0] = i
		rowMin := current[0]
		for j := 1; j <= len(br); j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}
			current[j] = min(min(current[j-1]+1, prev[j]+1), prev[j-1]+cost)
			if current[j] < rowMin {
				rowMin = current[j]
			}
		}
		if rowMin > limit {
			return false
		}
		prev = current
	}
	return prev[len(br)] <= limit
}

func longestCommonReasoningRunes(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 || len(br) == 0 {
		return 0
	}
	prev := make([]int, len(br)+1)
	best := 0
	for i := 1; i <= len(ar); i++ {
		current := make([]int, len(br)+1)
		for j := 1; j <= len(br); j++ {
			if ar[i-1] != br[j-1] {
				continue
			}
			current[j] = prev[j-1] + 1
			if current[j] > best {
				best = current[j]
			}
		}
		prev = current
	}
	return best
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
		m.clearActiveRunStartedAt()
		return m, nil
	}

	fullContent := []rune(m.streamingFullContent)
	m.streamingVisibleRunes = min(len(fullContent), m.streamingVisibleRunes+fakeStreamChunkSize(len(fullContent), m.streamingVisibleRunes))
	m.messages[m.streamingMessageIndex].Content = string(fullContent[:m.streamingVisibleRunes])
	m.refreshViewportForStreaming()

	if m.streamingVisibleRunes < len(fullContent) {
		return m, fakeStreamTickForContent(len(fullContent))
	}

	finishStatus := m.streamingFinishStatus
	finished := m.streamingFinished
	m.clearFakeStream()
	var notifyCmd tea.Cmd
	if finished {
		m.status = fallback(finishStatus, "ready")
		m.appendRunDurationStatus(finishStatus, time.Now())
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
	return terminalBellCmd()
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
	m.clearActiveRunStartedAt()
	m.refreshViewport()
}

func (m *model) finishFakeStreamContent() {
	if !m.streaming || m.streamingMessageIndex < 0 || m.streamingMessageIndex >= len(m.messages) {
		return
	}
	if m.streamingFullContent == "" {
		return
	}
	m.messages[m.streamingMessageIndex].Content = m.streamingFullContent
	m.streamingVisibleRunes = len([]rune(m.streamingFullContent))
}

func (m *model) clearFakeStream() {
	m.streaming = false
	m.streamingMessageIndex = 0
	m.streamingFullContent = ""
	m.streamingVisibleRunes = 0
	m.streamingFinished = false
	m.streamingFinishStatus = ""
	m.streamingMarkdownCache = streamingMarkdownCache{}
	m.streamingTranscriptCache = streamingTranscriptCache{}
}

func newRunID() string {
	return fmt.Sprintf("run_%d", time.Now().UnixNano())
}

func fakeStreamTick() tea.Cmd {
	return fakeStreamTickForContent(0)
}
func fakeStreamTickForContent(totalRunes int) tea.Cmd {
	return tea.Tick(fakeStreamTickInterval(totalRunes), func(time.Time) tea.Msg {
		return fakeStreamTickMsg{}
	})
}

func inputCursorPulseTick() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg {
		return inputCursorPulseMsg{}
	})
}

func fakeStreamTickInterval(totalRunes int) time.Duration {
	if totalRunes > 20000 {
		return 80 * time.Millisecond
	}
	if totalRunes > 8000 {
		return 50 * time.Millisecond
	}
	if totalRunes > 2000 {
		return 33 * time.Millisecond
	}
	return 18 * time.Millisecond
}
func fakeStreamChunkSize(totalRunes int, visibleRunes int) int {
	remaining := totalRunes - visibleRunes
	if remaining <= 0 {
		return 0
	}
	// Bound cosmetic fake streaming to roughly a couple of seconds. Long
	// responses are already complete in the runtime event, so drawing tiny chunks
	// only makes the TUI spend more time replacing viewport content.
	targetTicks := 90
	if totalRunes > 20000 {
		targetTicks = 25
	} else if totalRunes > 8000 {
		targetTicks = 45
	} else if totalRunes > 2000 {
		targetTicks = 65
	}
	size := max(3, (totalRunes+targetTicks-1)/targetTicks)
	return min(size, remaining)
}

func (m *model) appendRunDurationStatus(status string, now time.Time) bool {
	if m.activeRunStartedAt.IsZero() {
		return false
	}
	startedAt := m.activeRunStartedAt
	m.clearActiveRunStartedAt()
	if !shouldNotifyRunComplete(status) {
		return false
	}
	m.messages = append(m.messages, localStatusMessage(formatWorkedDuration(now.Sub(startedAt))))
	m.refreshViewport()
	return true
}

func (m *model) clearActiveRunStartedAt() {
	m.activeRunStartedAt = time.Time{}
}

func formatWorkedDuration(duration time.Duration) string {
	minutes := int(duration.Round(time.Minute) / time.Minute)
	if minutes < 1 {
		minutes = 1
	}

	hours := minutes / 60
	remainingMinutes := minutes % 60
	minuteLabel := func(value int) string {
		if value == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", value)
	}
	hourLabel := func(value int) string {
		if value == 1 {
			return "1 hr"
		}
		return fmt.Sprintf("%d hrs", value)
	}

	if hours == 0 {
		return "worked for " + minuteLabel(minutes)
	}
	if remainingMinutes == 0 {
		return "worked for " + hourLabel(hours)
	}
	return "worked for " + hourLabel(hours) + " & " + minuteLabel(remainingMinutes)
}
