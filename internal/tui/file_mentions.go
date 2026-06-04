package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

const maxFileMentionEntries = 3000

var fileMentionIgnoredDirs = map[string]bool{
	".git":         true,
	".qubit":       true,
	"bin":          true,
	"dist":         true,
	"node_modules": true,
	"vendor":       true,
}

type fileMentionToken struct {
	Start int
	End   int
	Query string
}

func (m model) showFileMentionPalette() bool {
	if m.mode != modeChat || m.busy || !m.ready || m.forkSelector.Active || m.showSlashPalette() {
		return false
	}
	_, ok := m.activeFileMentionToken()
	return ok
}

func (m model) activeFileMentionToken() (fileMentionToken, bool) {
	value := []rune(m.composer.Value())
	cursor := clampInt(m.composer.Cursor(), 0, len(value))
	if cursor == 0 {
		return fileMentionToken{}, false
	}
	start := cursor
	for start > 0 && !unicode.IsSpace(value[start-1]) {
		start--
	}
	if start >= cursor || value[start] != '@' {
		return fileMentionToken{}, false
	}
	query := string(value[start+1 : cursor])
	if strings.HasPrefix(query, "\"") {
		query = strings.TrimPrefix(query, "\"")
	}
	return fileMentionToken{Start: start, End: cursor, Query: query}, true
}

func (m *model) ensureFileMentionIndex() {
	cwd := m.fileMentionCwd()
	if cwd == "" {
		m.fileMention = fileMentionState{Err: "workspace cwd unavailable"}
		return
	}
	if m.fileMention.IndexedCwd == cwd && len(m.fileMention.Entries) > 0 {
		return
	}
	entries, err := scanFileMentionEntries(cwd, maxFileMentionEntries)
	m.fileMention = fileMentionState{Entries: entries, IndexedCwd: cwd}
	if err != nil {
		m.fileMention.Err = err.Error()
	}
}

func (m model) fileMentionCwd() string {
	if m.runtime != nil && m.runtime.launchCwd != "" {
		return m.runtime.launchCwd
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func scanFileMentionEntries(root string, limit int) ([]fileMentionEntry, error) {
	entries := make([]fileMentionEntry, 0, min(limit, 256))
	root = filepath.Clean(root)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			return nil
		}
		if path == root {
			return nil
		}
		name := d.Name()
		if d.IsDir() && fileMentionIgnoredDirs[name] {
			return filepath.SkipDir
		}
		if len(entries) >= limit {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			rel += "/"
		}
		entries = append(entries, fileMentionEntry{Path: rel, Name: name, IsDir: d.IsDir()})
		return nil
	})
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Path) < strings.ToLower(entries[j].Path)
	})
	return entries, err
}

func (m model) filteredFileMentionEntries() []fileMentionEntry {
	_, tokenOK := m.activeFileMentionToken()
	if !tokenOK {
		return nil
	}
	token, _ := m.activeFileMentionToken()
	query := strings.ToLower(strings.TrimSpace(token.Query))
	if query == "" {
		return append([]fileMentionEntry(nil), m.fileMention.Entries...)
	}

	var namePrefix []fileMentionEntry
	var nameContains []fileMentionEntry
	var pathContains []fileMentionEntry
	for _, entry := range m.fileMention.Entries {
		name := strings.ToLower(entry.Name)
		path := strings.ToLower(entry.Path)
		switch {
		case strings.HasPrefix(name, query):
			namePrefix = append(namePrefix, entry)
		case strings.Contains(name, query):
			nameContains = append(nameContains, entry)
		case strings.Contains(path, query):
			pathContains = append(pathContains, entry)
		}
	}
	matches := append(namePrefix, nameContains...)
	return append(matches, pathContains...)
}

func (m *model) moveFileMentionCursor(delta int) {
	m.ensureFileMentionIndex()
	matches := m.filteredFileMentionEntries()
	if len(matches) == 0 {
		m.fileMention.Cursor = 0
		return
	}
	m.fileMention.Cursor = (m.fileMention.Cursor + delta + len(matches)) % len(matches)
}

func (m model) acceptFileMentionSelection() (model, bool) {
	m.ensureFileMentionIndex()
	matches := m.filteredFileMentionEntries()
	if len(matches) == 0 {
		return m, false
	}
	token, ok := m.activeFileMentionToken()
	if !ok {
		return m, false
	}
	if m.fileMention.Cursor < 0 || m.fileMention.Cursor >= len(matches) {
		m.fileMention.Cursor = 0
	}
	entry := matches[m.fileMention.Cursor]
	display := "@" + quoteFileMentionPath(entry.Name)
	value := []rune(m.composer.Value())
	nextCursor := token.Start + len([]rune(display)) + 1
	next := string(value[:token.Start]) + display + " " + string(value[token.End:])
	m.composer.SetValue(next)
	m.composer.SetCursor(clampInt(nextCursor, 0, m.composer.Len()))
	m.composer.ClearSelection()
	m.composer.EnsureCursorVisible()
	m.fileMention.Cursor = 0
	m.rememberFileMentionSelection(display, "@"+quoteFileMentionPath(entry.Path))
	return m, true
}

func (m *model) rememberFileMentionSelection(display string, path string) {
	for i, selection := range m.fileMention.Selections {
		if selection.Display == display {
			m.fileMention.Selections[i].Path = path
			return
		}
	}
	m.fileMention.Selections = append(m.fileMention.Selections, fileMentionSelection{Display: display, Path: path})
}

func (m model) expandFileMentionsForSend(input string) string {
	for _, selection := range m.fileMention.Selections {
		if selection.Display == "" || selection.Path == "" || selection.Display == selection.Path {
			continue
		}
		input = replaceMentionToken(input, selection.Display, selection.Path)
	}
	m.fileMention.Selections = nil
	return input
}

func replaceMentionToken(input string, display string, path string) string {
	if display == "" {
		return input
	}
	var b strings.Builder
	for i := 0; i < len(input); {
		idx := strings.Index(input[i:], display)
		if idx < 0 {
			b.WriteString(input[i:])
			break
		}
		idx += i
		end := idx + len(display)
		if isMentionBoundary(input, idx-1) && isMentionBoundary(input, end) {
			b.WriteString(input[i:idx])
			b.WriteString(path)
			i = end
			continue
		}
		b.WriteString(input[i:end])
		i = end
	}
	return b.String()
}

func isMentionBoundary(input string, index int) bool {
	if index < 0 || index >= len(input) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(input[index:])
	return unicode.IsSpace(r)
}

func quoteFileMentionPath(path string) string {
	if strings.IndexFunc(path, unicode.IsSpace) < 0 {
		return path
	}
	return "\"" + strings.ReplaceAll(path, "\"", "\\\"") + "\""
}

func (m model) fileMentionModalHeight(maxHeight int) int {
	matches := m.filteredFileMentionEntries()
	visibleOptions := min(7, len(matches))
	if visibleOptions == 0 {
		return min(max(4, maxHeight), 5)
	}
	return min(max(4, maxHeight), visibleOptions+8)
}

func (m model) renderFileMentionModal(height int) string {
	matches := m.filteredFileMentionEntries()
	modal := modalState{
		Title:       "Files",
		Description: "Tab/Enter to insert a file mention.",
		Options:     fileMentionModalOptions(matches),
	}
	if m.fileMention.Err != "" {
		modal.Description = fmt.Sprintf("File scan warning: %s", m.fileMention.Err)
	}
	if len(matches) == 0 {
		modal.Description = "No matching files."
	}
	if m.fileMention.Cursor < 0 || m.fileMention.Cursor >= len(matches) {
		modal.OptionCursor = 0
	} else {
		modal.OptionCursor = m.fileMention.Cursor
	}
	return m.renderModalStateAligned(modal, height, lipgloss.Left, lipgloss.Bottom)
}

func fileMentionModalOptions(entries []fileMentionEntry) []modalOption {
	options := make([]modalOption, 0, len(entries))
	for _, entry := range entries {
		description := "file"
		if entry.IsDir {
			description = "dir"
		}
		options = append(options, modalOption{ID: entry.Path, Label: entry.Path, Description: description})
	}
	return options
}
