package tui

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
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
		if err := writeClipboard(text); err != nil {
			return uiErrMsg{err: fmt.Errorf("copy selection: %w", err)}
		}
		return nil
	}
}

func writeClipboard(text string) error {
	if err := clipboard.WriteAll(text); err == nil {
		return nil
	} else if oscErr := writeOSC52Clipboard(text); oscErr != nil {
		return fmt.Errorf("%w; OSC52 fallback failed: %v", err, oscErr)
	}
	return nil
}

func writeOSC52Clipboard(text string) error {
	seq := osc52.New(text).String()
	if seq == "" {
		return fmt.Errorf("OSC52 sequence was empty")
	}
	_, err := fmt.Fprint(os.Stderr, seq)
	return err
}
