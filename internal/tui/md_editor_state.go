package tui

type mdEditorView string

const (
	mdEditorList           mdEditorView = "list"
	mdEditorEdit           mdEditorView = "edit"
	mdEditorRename         mdEditorView = "rename"
	mdEditorDiscardConfirm mdEditorView = "discard-confirm"
)

type mdEditorState struct {
	View            mdEditorView
	Loading         bool
	Files           []mdFileInfo
	Cursor          int
	Current         *mdFileInfo
	Editor          composerModel
	OriginalContent string
	Dirty           bool
	Status          string
	Rename          composerModel
	ConfirmCursor   int
}
