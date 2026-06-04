package tui

import "charm.land/bubbles/v2/viewport"

type mdEditorView string

const (
	mdEditorList           mdEditorView = "list"
	mdEditorPreview        mdEditorView = "preview"
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
	Preview         viewport.Model
	PreviewContent  string
	PreviewLines    []transcriptRenderLine
	PreviewSelect   transcriptSelectionState
	OriginalContent string
	Dirty           bool
	Status          string
	Rename          composerModel
	ConfirmCursor   int
}
