package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func newMdEditorState() mdEditorState {
	return mdEditorState{View: mdEditorList, Loading: true, Editor: newMdDocumentComposer(), Rename: newMdRenameComposer()}
}

func newMdDocumentComposer() composerModel {
	c := newComposer()
	c.SetPlaceholder("write markdown...")
	c.SetMaxHeight(1000)
	c.SetMaxContentHeight(4000)
	return c
}

func newMdRenameComposer() composerModel {
	c := newComposer()
	c.SetPlaceholder("filename")
	c.SetMaxHeight(1)
	return c
}

func (m model) openMdEditor() (tea.Model, tea.Cmd) {
	m.mode = modeMdEditor
	m.previousMode = modeChat
	m.mdEditor = newMdEditorState()
	m.mdEditor.Loading = true
	m.status = "loading markdown docs"
	return m, sendRuntime(m.runtime, map[string]any{"type": "md.list"})
}

func (m model) closeMdEditor() model {
	m.mode = modeChat
	m.mdEditor = mdEditorState{}
	if !m.inputSpinnerActive() {
		m.status = "ready"
	}
	return m
}

func (m model) updateMdEditor(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.mdEditor.View {
	case mdEditorEdit:
		return m.updateMdEditorEdit(msg)
	case mdEditorRename:
		return m.updateMdEditorRename(msg)
	case mdEditorDiscardConfirm:
		return m.updateMdEditorDiscardConfirm(msg)
	default:
		return m.updateMdEditorList(msg)
	}
}

func (m model) updateMdEditorList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m.closeMdEditor(), nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k", "ctrl+p":
		m.moveMdEditorCursor(-1)
		return m, nil
	case "down", "j", "ctrl+n":
		m.moveMdEditorCursor(1)
		return m, nil
	case "n", "N":
		m.mdEditor.Loading = true
		m.mdEditor.Status = "creating user doc"
		m.status = "creating markdown doc"
		return m, sendRuntime(m.runtime, map[string]any{"type": "md.create"})
	case "enter":
		if len(m.mdEditor.Files) == 0 {
			return m, nil
		}
		m.ensureMdEditorCursor()
		file := m.mdEditor.Files[m.mdEditor.Cursor]
		m.mdEditor.Loading = true
		m.mdEditor.Status = "opening " + file.Name + ".md"
		m.status = "opening markdown doc"
		return m, sendRuntime(m.runtime, map[string]any{"type": "md.read", "path": file.Path})
	}
	return m, nil
}

func (m model) updateMdEditorEdit(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Keep the editor's visible height in sync before navigation. Without this,
	// composerModel may still think it has its default single-line height and
	// scroll as soon as Up/Down is pressed, even when the cursor is inside the
	// visible full-screen editor area.
	m.layoutMdEditorComposer()
	if isMdEditorNewlineKey(msg) {
		m.mdEditor.Editor.InsertNewline()
		m.updateMdEditorDirty()
		m.layoutMdEditorComposer()
		return m, nil
	}

	switch msg.String() {
	case "ctrl+s":
		if m.mdEditor.Current == nil {
			return m, nil
		}
		m.mdEditor.Loading = true
		m.mdEditor.Status = "saving " + m.mdEditor.Current.Name + ".md"
		m.status = "saving markdown doc"
		return m, sendRuntime(m.runtime, map[string]any{"type": "md.save", "path": m.mdEditor.Current.Path, "content": m.mdEditor.Editor.Value()})
	case "ctrl+r":
		return m.openMdEditorRename(), nil
	case "esc":
		if m.mdEditor.Dirty {
			m.mdEditor.View = mdEditorDiscardConfirm
			m.mdEditor.ConfirmCursor = 1
			m.mdEditor.Status = "discard unsaved changes?"
			m.status = "confirm discard"
			return m, nil
		}
		m.mdEditor.View = mdEditorList
		m.mdEditor.Current = nil
		m.mdEditor.Status = ""
		m.status = "markdown docs"
		return m, nil
	case "ctrl+c":
		if m.mdEditor.Editor.HasSelection() {
			m.mdEditor.Status = "copied selection"
			return m, copyClipboardCmd(m.mdEditor.Editor.SelectedText())
		}
		return m, tea.Quit
	}

	handled, cmd := m.mdEditor.Editor.UpdateKey(msg)
	if handled {
		m.updateMdEditorDirty()
		m.layoutMdEditorComposer()
		return m, cmd
	}
	return m, nil
}

func (m model) updateMdEditorTeaPaste(msg tea.PasteMsg) model {
	return m.insertMdEditorPaste(msg.Content)
}

func (m model) updateMdEditorPaste(msg composerPasteMsg) model {
	if msg.Err != nil {
		m.mdEditor.Status = "paste failed"
		return m
	}
	return m.insertMdEditorPaste(msg.Text)
}

func (m model) insertMdEditorPaste(text string) model {
	if text == "" {
		return m
	}
	switch m.mdEditor.View {
	case mdEditorEdit:
		m.layoutMdEditorComposer()
		m.mdEditor.Editor.InsertString(normalizeInputNewlines(text))
		m.updateMdEditorDirty()
		m.layoutMdEditorComposer()
		m.mdEditor.Status = "pasted"
	case mdEditorRename:
		m.layoutMdEditorComposer()
		m.mdEditor.Rename.InsertString(strings.TrimSpace(normalizeInputNewlines(text)))
		m.layoutMdEditorComposer()
		m.mdEditor.Status = "pasted"
	}
	return m
}

func (m model) openMdEditorRename() model {
	if m.mdEditor.Current == nil {
		return m
	}
	m.mdEditor.View = mdEditorRename
	m.mdEditor.Rename = newMdRenameComposer()
	m.mdEditor.Rename.SetValue(m.mdEditor.Current.Name)
	m.mdEditor.Status = "rename markdown file"
	m.status = "rename markdown file"
	return m
}

func isMdEditorNewlineKey(msg tea.KeyPressMsg) bool {
	if isNewlineKey(msg) {
		return true
	}
	keyEvent := msg.Key()
	return keyEvent.Code == tea.KeyEnter && keyEvent.Mod == 0
}

func (m model) updateMdEditorRename(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mdEditor.View = mdEditorEdit
		m.mdEditor.Status = ""
		m.status = "editing markdown"
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		if m.mdEditor.Current == nil {
			m.mdEditor.View = mdEditorEdit
			return m, nil
		}
		name := strings.TrimSpace(m.mdEditor.Rename.Value())
		if name == "" {
			m.mdEditor.Status = "filename is required"
			return m, nil
		}
		m.mdEditor.Loading = true
		m.mdEditor.Status = "renaming markdown file"
		m.status = "renaming markdown file"
		return m, sendRuntime(m.runtime, map[string]any{"type": "md.rename", "path": m.mdEditor.Current.Path, "name": name})
	}
	handled, cmd := m.mdEditor.Rename.UpdateKey(msg)
	if handled {
		return m, cmd
	}
	return m, nil
}

func (m model) updateMdEditorDiscardConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mdEditor.View = mdEditorEdit
		m.mdEditor.Status = ""
		m.status = "editing markdown"
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "left", "shift+tab", "right", "tab":
		m.mdEditor.ConfirmCursor = 1 - clampInt(m.mdEditor.ConfirmCursor, 0, 1)
		return m, nil
	case "enter":
		if m.mdEditor.ConfirmCursor == 0 {
			m.mdEditor.Editor.SetValue(m.mdEditor.OriginalContent)
			m.mdEditor.Dirty = false
			m.mdEditor.Current = nil
			m.mdEditor.View = mdEditorList
			m.mdEditor.Status = "discarded changes"
			m.status = "markdown docs"
			return m, nil
		}
		m.mdEditor.View = mdEditorEdit
		m.mdEditor.Status = ""
		m.status = "editing markdown"
		return m, nil
	}
	return m, nil
}

func (m model) updateMdEditorMouseWheel(msg tea.MouseWheelMsg) model {
	if m.mdEditor.View != mdEditorList {
		return m
	}
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		m.moveMdEditorCursor(-1)
	case tea.MouseWheelDown:
		m.moveMdEditorCursor(1)
	}
	return m
}

func (m *model) moveMdEditorCursor(delta int) {
	if len(m.mdEditor.Files) == 0 {
		m.mdEditor.Cursor = 0
		return
	}
	m.mdEditor.Cursor = (m.mdEditor.Cursor + delta + len(m.mdEditor.Files)) % len(m.mdEditor.Files)
}

func (m *model) ensureMdEditorCursor() {
	if len(m.mdEditor.Files) == 0 {
		m.mdEditor.Cursor = 0
		return
	}
	m.mdEditor.Cursor = clampInt(m.mdEditor.Cursor, 0, len(m.mdEditor.Files)-1)
}

func (m *model) updateMdEditorDirty() {
	m.mdEditor.Dirty = m.mdEditor.Editor.Value() != m.mdEditor.OriginalContent
}

func (m *model) layoutMdEditorComposer() {
	width := max(20, m.width-8)
	m.mdEditor.Editor.SetMaxHeight(m.mdEditorVisibleEditorHeight())
	m.mdEditor.Editor.SetWidth(width)
	m.mdEditor.Rename.SetWidth(max(12, width/2))
}

func (m model) mdEditorVisibleEditorHeight() int {
	bodyHeight := m.mdEditorBodyHeight()
	headerLines := renderedLineCount(m.mdEditorEditHeader())
	return max(1, bodyHeight-headerLines-3)
}

func (m model) mdEditorBodyHeight() int {
	if m.height <= 0 {
		return 20
	}
	queuedStatus := m.renderQueuedStatus()
	input := m.renderInput()
	status := m.renderInputStatus()
	footer := m.renderFooter()
	preOverlayBottomHeight := lipgloss.Height(queuedStatus) + lipgloss.Height(input) + lipgloss.Height(status) + lipgloss.Height(footer)
	bottomOverlay := m.renderBottomOverlay(max(0, min(maxBottomOverlayRows(m), m.height-preOverlayBottomHeight-4)))
	bottomHeight := preOverlayBottomHeight + lipgloss.Height(bottomOverlay)
	mainHeight := max(0, m.height-bottomHeight)
	return max(1, mainHeight-lipgloss.Height(m.renderHeader()))
}

func (m *model) applyMdList(ev runtimeEvent) {
	m.mdEditor.Files = ev.Files
	m.mdEditor.Loading = false
	m.ensureMdEditorCursor()
	m.mdEditor.Status = fmt.Sprintf("%d %s", len(ev.Files), plural(len(ev.Files), "markdown file", "markdown files"))
	m.status = "markdown docs"
}

func (m *model) applyMdRead(ev runtimeEvent) {
	file := ev.File
	if file == nil {
		file = &mdFileInfo{Name: "untitled", Path: ev.Path}
	}
	m.openMdEditorFile(*file, ev.Content)
}

func (m *model) applyMdCreated(ev runtimeEvent) {
	file := ev.File
	if file == nil {
		file = &mdFileInfo{Name: "untitled", Section: "user-docs", Path: ev.Path}
	}
	m.upsertMdEditorFile(*file)
	m.openMdEditorFile(*file, ev.Content)
	m.mdEditor.Status = fallback(ev.Status, "created "+file.Name+".md")
}

func (m *model) applyMdSaved(ev runtimeEvent) {
	if ev.File != nil {
		m.upsertMdEditorFile(*ev.File)
		m.mdEditor.Current = cloneMdFileInfo(ev.File)
	}
	m.mdEditor.OriginalContent = ev.Content
	m.mdEditor.Dirty = false
	m.mdEditor.Loading = false
	m.mdEditor.Status = fallback(ev.Status, "saved")
	m.status = "editing markdown"
}

func (m *model) applyMdRenamed(ev runtimeEvent) {
	if ev.File == nil {
		return
	}
	oldPath := ev.Path
	for i := range m.mdEditor.Files {
		if m.mdEditor.Files[i].Path == oldPath || (m.mdEditor.Current != nil && m.mdEditor.Files[i].Path == m.mdEditor.Current.Path) {
			m.mdEditor.Files[i] = *ev.File
			m.mdEditor.Current = cloneMdFileInfo(ev.File)
			m.mdEditor.Loading = false
			m.mdEditor.View = mdEditorEdit
			m.mdEditor.Status = fallback(ev.Status, "renamed")
			m.status = "editing markdown"
			return
		}
	}
	m.upsertMdEditorFile(*ev.File)
	m.mdEditor.Current = cloneMdFileInfo(ev.File)
	m.mdEditor.Loading = false
	m.mdEditor.View = mdEditorEdit
	m.mdEditor.Status = fallback(ev.Status, "renamed")
	m.status = "editing markdown"
}

func (m *model) openMdEditorFile(file mdFileInfo, content string) {
	m.mdEditor.Current = &file
	m.mdEditor.Editor = newMdDocumentComposer()
	m.layoutMdEditorComposer()
	m.mdEditor.Editor.SetValue(content)
	m.mdEditor.OriginalContent = content
	m.mdEditor.Dirty = false
	m.mdEditor.View = mdEditorEdit
	m.mdEditor.Loading = false
	m.mdEditor.Status = ""
	m.status = "editing markdown"
}

func (m *model) upsertMdEditorFile(file mdFileInfo) {
	for i := range m.mdEditor.Files {
		if m.mdEditor.Files[i].Path == file.Path || (m.mdEditor.Files[i].Section == file.Section && m.mdEditor.Files[i].Name == file.Name) {
			m.mdEditor.Files[i] = file
			return
		}
	}
	m.mdEditor.Files = append([]mdFileInfo{file}, m.mdEditor.Files...)
	m.mdEditor.Cursor = 0
}

func cloneMdFileInfo(file *mdFileInfo) *mdFileInfo {
	if file == nil {
		return nil
	}
	clone := *file
	return &clone
}

func (m model) renderMdEditor(height int) string {
	switch m.mdEditor.View {
	case mdEditorEdit:
		return m.renderMdEditorEdit(height)
	case mdEditorRename:
		return m.renderMdEditorRename(height)
	case mdEditorDiscardConfirm:
		return m.renderMdEditorDiscardConfirm(height)
	default:
		return m.renderMdEditorList(height)
	}
}

func (m model) renderMdEditorList(height int) string {
	panelWidth := max(20, m.width-4)
	contentWidth := max(20, panelWidth-4)
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Bold(true).Render("markdown editor") + "\n")
	b.WriteString(mutedSt.Render("↑/↓ select · enter open · n new user doc · esc close") + "\n")
	if m.mdEditor.Status != "" {
		b.WriteString(mutedSt.Render(m.mdEditor.Status) + "\n")
	}
	b.WriteString("\n")
	if m.mdEditor.Loading {
		b.WriteString(mutedSt.Render("loading markdown files..."))
		return lipgloss.NewStyle().Padding(1, 2).Width(panelWidth).Render(b.String())
	}
	if len(m.mdEditor.Files) == 0 {
		b.WriteString(mutedSt.Render("no markdown files yet · press n to create a user doc"))
		return lipgloss.NewStyle().Padding(1, 2).Width(panelWidth).Render(b.String())
	}
	maxRows := max(1, height-6)
	window := visibleListWindow(len(m.mdEditor.Files), m.mdEditor.Cursor, maxRows)
	if window.HasAbove {
		b.WriteString(mutedSt.Render(fmt.Sprintf("  more above (%d)", window.Start)) + "\n")
	}
	for i := window.Start; i < window.End; i++ {
		if i == window.Start || m.mdEditor.Files[i].Section != m.mdEditor.Files[i-1].Section {
			if i != window.Start {
				b.WriteString("\n")
			}
			b.WriteString(renderMdSectionHeader(m.mdEditor.Files[i].Section) + "\n")
		}
		line := renderMdFileRow(m.mdEditor.Files[i], contentWidth-2)
		if i == m.mdEditor.Cursor {
			line = selectSt.Render("› ") + selectSt.Render(line)
		} else {
			line = mutedSt.Render("  ") + line
		}
		b.WriteString(line)
		if i < window.End-1 || window.HasBelow {
			b.WriteString("\n")
		}
	}
	if window.HasBelow {
		b.WriteString(mutedSt.Render(fmt.Sprintf("  more below (%d)", len(m.mdEditor.Files)-window.End)))
	}
	return lipgloss.NewStyle().Padding(1, 2).Width(panelWidth).Render(b.String())
}

func renderMdSectionHeader(section string) string {
	switch section {
	case "user-docs":
		return mutedSt.Render("User notes")
	case "plans":
		return mutedSt.Render("Agent plans")
	default:
		return mutedSt.Render(section)
	}
}

func renderMdFileRow(file mdFileInfo, width int) string {
	label := file.Name + ".md"
	if strings.TrimSpace(file.Title) != "" {
		label += " · " + file.Title
	}
	return lipgloss.NewStyle().Foreground(cyan).Render(oneLine(label, max(8, width)))
}

func (m model) renderMdEditorEdit(height int) string {
	header := m.mdEditorEditHeader()
	headerHeight := renderedLineCount(header) + 1
	editorHeight := max(1, height-headerHeight-2)
	editorModel := m.mdEditor.Editor
	editorModel.SetMaxHeight(editorHeight)
	editorModel.SetWidth(max(20, m.width-8))
	editor := editorModel.ViewStyled("", m.inputCursorPulse, lipgloss.Style{})
	body := header + "\n\n" + renderFixedHeight(editor, editorHeight)
	return lipgloss.NewStyle().Padding(1, 2).Width(max(20, m.width-4)).Render(body)
}

func (m model) mdEditorEditHeader() string {
	fileLabel := "untitled.md"
	if m.mdEditor.Current != nil {
		fileLabel = m.mdEditor.Current.Section + "/" + m.mdEditor.Current.Name + ".md"
	}
	dirty := ""
	if m.mdEditor.Dirty {
		dirty = mutedSt.Render(" · modified")
	}
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("md editor") + mutedSt.Render(" · ") + lipgloss.NewStyle().Foreground(cyan).Render(fileLabel) + dirty
	if m.mdEditor.Status != "" {
		header += "\n" + mutedSt.Render(m.mdEditor.Status)
	}
	return header
}

func (m model) renderMdEditorRename(height int) string {
	base := m.renderMdEditorEdit(max(1, height-4))
	prompt := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("rename") + "\n" + m.mdEditor.Rename.ViewStyled(idleInputPrompt(), m.inputCursorPulse, lipgloss.Style{}) + "\n" + mutedSt.Render("enter rename · esc cancel")
	return renderFixedHeight(base+"\n\n"+prompt, height)
}

func (m model) renderMdEditorDiscardConfirm(height int) string {
	base := m.renderMdEditorEdit(max(1, height-5))
	actions := []string{"Discard", "Cancel"}
	var rendered []string
	for i, action := range actions {
		button := "[ " + action + " ]"
		if i == m.mdEditor.ConfirmCursor {
			button = selectSt.Render(button)
		} else if i == 0 {
			button = errSt.Render(button)
		} else {
			button = mutedSt.Render(button)
		}
		rendered = append(rendered, button)
	}
	confirm := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("Unsaved changes") + "\n" + mutedSt.Render("Discard changes and return to the list?") + "\n\n" + strings.Join(rendered, "  ")
	return renderFixedHeight(base+"\n\n"+confirm, height)
}
