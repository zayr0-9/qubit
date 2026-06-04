package tui

type modalState struct {
	ID           string
	Kind         modalKind
	Title        string
	Description  string
	Fields       []modalField
	Options      []modalOption
	OptionCursor int
	Actions      []modalAction
	Cursor       int
	Payload      map[string]any
}
