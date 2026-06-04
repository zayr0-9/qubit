package tui

type transcriptSelectionPoint struct {
	Line int
	Col  int
}

type transcriptSelectionState struct {
	Active       bool
	Dragging     bool
	Anchor       transcriptSelectionPoint
	Cursor       transcriptSelectionPoint
	MouseDownX   int
	MouseDownY   int
	PendingClick bool
}

type transcriptRenderLine struct {
	Text       string
	Selectable bool
}

type linkHitbox struct {
	URL    string
	Line   int
	StartX int
	EndX   int
}
