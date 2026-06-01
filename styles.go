package main

import "charm.land/lipgloss/v2"

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

	appStyle    = lipgloss.NewStyle().Background(bg).Foreground(text)
	headerStyle = lipgloss.NewStyle().Background(bg).Foreground(text).Padding(0, 2)
	chatStyle   = lipgloss.NewStyle().Background(bg).Foreground(text).Padding(0, 2)
	inputStyle  = lipgloss.NewStyle().Background(surface).Foreground(text).Padding(0, 2)
	footerStyle = lipgloss.NewStyle().Background(bg).PaddingLeft(2)
	userName    = lipgloss.NewStyle().Foreground(accent).Bold(true)
	aiName      = lipgloss.NewStyle().Foreground(cyan).Bold(true)
	mutedSt     = lipgloss.NewStyle().Foreground(muted)
	errSt       = lipgloss.NewStyle().Foreground(red)
	okSt        = lipgloss.NewStyle().Foreground(green)
	selectSt    = lipgloss.NewStyle().Foreground(text).Background(surfaceHi).Bold(true)
)
