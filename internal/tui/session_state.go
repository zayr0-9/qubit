package tui

type forkPoint struct {
	Number           int
	MessageIndex     int
	EditMessageIndex int
	Content          string
}

type forkSelectorState struct {
	Active bool
	Points []forkPoint
	Cursor int
}

type messageEditState struct {
	Active       bool
	MessageIndex int
	Original     string
}
