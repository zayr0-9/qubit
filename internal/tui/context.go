package tui

import (
	"encoding/json"
	"fmt"
	"strings"
)

const charsPerToken = 4

func (m model) contextStatusText() string {
	maxTokens := m.activeModelMaxContext()
	if maxTokens <= 0 {
		return ""
	}
	if usage := m.codexUsageForStatus(); usage != nil {
		usedTokens := usage.InputTokens + usage.OutputTokens
		if usedTokens <= 0 {
			usedTokens = usage.TotalTokens
		}
		if usedTokens > 0 {
			status := fmt.Sprintf("ctx %s/%s", formatTokenCount(usedTokens), formatTokenCount(maxTokens))
			if usage.CachedTokens > 0 {
				status += fmt.Sprintf(" cache %s", formatTokenCount(usage.CachedTokens))
			}
			return status
		}
	}
	usedTokens := estimateContextTokens(m.messages)
	return fmt.Sprintf("ctx %s/%s", formatTokenCount(usedTokens), formatTokenCount(maxTokens))
}

func (m model) codexUsageForStatus() *codexUsage {
	// Loaded transcripts are the durable source of truth for session status. Prefer
	// the latest message-level Codex usage when present so reopening an older
	// session cannot fall back to the local context estimator or a stale live run.
	usage := latestCodexUsageFromMessages(m.messages)
	if usage == nil {
		usage = m.lastCodexUsage
	}
	if !hasCodexUsageTokens(usage) {
		return nil
	}
	return usage
}

func hasCodexUsageTokens(usage *codexUsage) bool {
	if usage == nil {
		return false
	}
	return usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.TotalTokens > 0 || usage.CachedTokens > 0
}

func latestCodexUsageFromMessages(messages []chatMessage) *codexUsage {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].CodexUsage != nil {
			return messages[i].CodexUsage
		}
	}
	return nil
}

func (m model) activeModelMaxContext() int {
	if m.maxContext > 0 {
		return m.maxContext
	}
	for _, info := range m.models {
		if info.ID == m.model && info.MaxContext > 0 {
			return info.MaxContext
		}
	}
	return 0
}

func estimateContextTokens(messages []chatMessage) int {
	chars := 0
	for _, message := range messages {
		chars += len([]rune(message.Role))
		chars += len([]rune(message.Content))
		chars += len([]rune(message.ReasoningContent))
		if message.ToolGroup != nil {
			chars += len([]rune(toolGroupContextText(message.ToolGroup)))
		}
	}
	if chars <= 0 {
		return 0
	}
	return (chars + charsPerToken - 1) / charsPerToken
}

func toolGroupContextText(group *toolGroup) string {
	if group == nil {
		return ""
	}
	chars := 0
	hasContextChars := false
	for _, call := range group.Calls {
		if call.ContextChars > 0 {
			hasContextChars = true
			chars += call.ContextChars
		}
	}
	if hasContextChars {
		return strings.Repeat("x", chars)
	}
	data, err := json.Marshal(group)
	if err != nil {
		return toolGroupLabel(group)
	}
	return string(data)
}

func formatTokenCount(tokens int) string {
	if tokens >= 1000 {
		value := float64(tokens) / 1000
		formatted := fmt.Sprintf("%.1f", value)
		return strings.TrimSuffix(formatted, ".0") + "k"
	}
	return fmt.Sprintf("%d", tokens)
}
