package main

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
	usedTokens := estimateContextTokens(m.messages)
	status := fmt.Sprintf("ctx %s/%s", formatTokenCount(usedTokens), formatTokenCount(maxTokens))
	if usage := m.codexUsageStatusText(); usage != "" {
		status += " " + usage
	}
	return status
}

func (m model) codexUsageStatusText() string {
	if m.lastCodexUsage == nil || !isCodexProvider(m.activeProvider, m.provider) {
		return ""
	}
	parts := []string{}
	if m.lastCodexUsage.InputTokens > 0 {
		parts = append(parts, fmt.Sprintf("in %s", formatTokenCount(m.lastCodexUsage.InputTokens)))
	}
	if m.lastCodexUsage.CachedTokens > 0 {
		parts = append(parts, fmt.Sprintf("cache %s", formatTokenCount(m.lastCodexUsage.CachedTokens)))
	}
	if m.lastCodexUsage.OutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("out %s", formatTokenCount(m.lastCodexUsage.OutputTokens)))
	}
	if len(parts) == 0 && m.lastCodexUsage.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("total %s", formatTokenCount(m.lastCodexUsage.TotalTokens)))
	}
	if len(parts) == 0 {
		return ""
	}
	return "log " + strings.Join(parts, "/")
}

func isCodexProvider(activeProvider, provider string) bool {
	return strings.EqualFold(activeProvider, "codex") || strings.EqualFold(provider, "codex")
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
