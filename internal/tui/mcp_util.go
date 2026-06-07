package tui

import "strings"

func isMcpToolName(name string) bool {
	return strings.HasPrefix(name, "mcp__")
}
