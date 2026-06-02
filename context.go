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
	return fmt.Sprintf("ctx %s/%s", formatTokenCount(usedTokens), formatTokenCount(maxTokens))
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
	data, err := json.Marshal(group)
	if err != nil {
		return toolGroupLabel(group)
	}
	return string(data)
}

func formatTokenCount(tokens int) string {
	if tokens >= 1000 {
		value := float64(tokens) / 1000
		formatted := fmt.Sprintf("%.1fk", value)
		return strings.TrimSuffix(formatted, ".0k") + "k"
	}
	return fmt.Sprintf("%d", tokens)
}
