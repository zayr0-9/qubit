package tui

type themeEntryStep int

const (
	themeEntryPresets themeEntryStep = iota
	themeEntryBackground
	themeEntryText
)

type themeEntryState struct {
	Step       themeEntryStep
	Preset     int
	Background composerModel
	Text       composerModel
	Err        string
}
