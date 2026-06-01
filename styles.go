package main

import (
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

var (
	bg        = lipgloss.Color("#101112")
	surface   = lipgloss.Color("#17191b")
	surfaceHi = lipgloss.Color("#202326")
	muted     = lipgloss.Color("#7c838a")
	text      = lipgloss.Color("#e6e8eb")
	accent    = lipgloss.Color("#f2a65a")
	cyan      = lipgloss.Color("#8bd3dd")
	red       = lipgloss.Color("#ff6b6b")
	green     = lipgloss.Color("#9be28f")

	appStyle         = lipgloss.NewStyle().Foreground(text)
	headerStyle      = lipgloss.NewStyle().Foreground(text).Padding(0, 2)
	chatStyle        = lipgloss.NewStyle().Foreground(text).Padding(0, 1)
	inputStyle       = lipgloss.NewStyle().Background(surface).Foreground(text).PaddingRight(2)
	footerStyle      = lipgloss.NewStyle().Foreground(muted).PaddingLeft(2)
	userIcon         = lipgloss.NewStyle().Foreground(accent).Bold(true)
	aiIcon           = lipgloss.NewStyle().Foreground(cyan).Bold(true)
	errorIcon        = lipgloss.NewStyle().Foreground(red).Bold(true)
	mutedSt          = lipgloss.NewStyle().Foreground(muted)
	errSt            = lipgloss.NewStyle().Foreground(red)
	okSt             = lipgloss.NewStyle().Foreground(green)
	selectSt         = lipgloss.NewStyle().Foreground(text).Background(surfaceHi).Bold(true)
	inputSelectSt    = lipgloss.NewStyle().Foreground(bg).Background(accent)
	composerCursorSt = lipgloss.NewStyle().Foreground(bg).Background(text)
)

var (
	inputNewlineBinding = key.NewBinding(key.WithKeys("shift+enter", "alt+enter", "ctrl+j"))
)
