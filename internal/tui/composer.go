package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/qubit/graviton-cli/internal/tui/components/composer"
)

type composerModel = composer.Model
type composerPasteMsg = composer.PasteMsg

func newComposer() composerModel {
	composer.ConfigureStyles(mutedSt, inputSelectSt, composerCursorStyles)
	return composer.New()
}

func copyClipboardCmd(text string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.WriteAll(text); err != nil {
			return runtimeErrMsg{err: fmt.Errorf("copy selection: %w", err)}
		}
		return nil
	}
}
