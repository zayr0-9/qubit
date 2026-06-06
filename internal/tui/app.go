package tui

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

func initialModel(rt *runtimeClient) model {
	composer := newComposer()
	theme := defaultTheme()
	if loadedTheme, err := loadThemeConfig(runtimeQubitDir(rt)); err == nil && loadedTheme.Background != "" && loadedTheme.Text != "" {
		theme = loadedTheme
	}
	inputHistory := []string(nil)
	if rt != nil {
		var err error
		inputHistory, err = loadInputHistory(rt.qubitDir)
		if err != nil {
			inputHistory = nil
		}
	}
	applyTheme(theme)

	spin := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(spinnerStyle))

	return model{
		viewport:             viewport.New(),
		composer:             composer,
		spinner:              spin,
		renderCache:          make(map[renderCacheKey]string),
		markdownRenderers:    make(markdownRendererCache),
		messages:             []chatMessage{{Role: "assistant", Content: "Ready. Try / for commands."}},
		status:               "starting runtime",
		runtimeConnected:     rt != nil,
		permissionMode:       permissionModeAsk,
		cwdBlockEnabled:      true,
		theme:                theme,
		autoNewSessionOnChat: true,
		autoScroll:           true,
		activeReasoningIndex: -1,
		activeReasoningStart: -1,
		inputHistory:         inputHistory,
		inputHistoryIndex:    len(inputHistory),
		notifier:             newPlatformNotifier(),
		runtime:              rt,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(waitRuntimeEvent(m.runtime), m.spinner.Tick, inputCursorPulseTick())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil
	case tea.KeyboardEnhancementsMsg:
		m.keyboardEnhanced = msg.SupportsKeyDisambiguation()
		return m, nil
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	case tea.MouseWheelMsg:
		return m.updateMouseWheelRouted(msg), nil
	case tea.MouseClickMsg:
		return m.updateMouseClick(msg), nil
	case tea.MouseMotionMsg:
		return m.updateMouseMotion(msg), nil
	case tea.MouseReleaseMsg:
		return m.updateMouseRelease(msg)
	case runtimeMsg:
		return m.updateRuntime(runtimeEvent(msg))
	case runtimeErrMsg:
		return m.updateRuntimeError(msg.err)
	case uiErrMsg:
		return m.updateUIError(msg.err), nil
	case runtimeReconnectMsg:
		return m.updateRuntimeReconnect(msg.err)
	case sendDoneMsg:
		return m.updateSendDone(msg.err), nil
	case terminalSetupResultMsg:
		return m.updateTerminalSetupResult(terminalSetupResult(msg)), nil
	case fakeStreamTickMsg:
		return m.updateFakeStreamTick()
	case inputCursorPulseMsg:
		m.inputCursorPulse++
		if m.hasRunningToolGroup() {
			m.refreshViewport()
		}
		return m, inputCursorPulseTick()
	case toolCallRevealTickMsg:
		return m.updateToolCallRevealTick()
	case notificationResultMsg:
		return m.updateNotificationResult(msg), nil
	case terminalBellResultMsg:
		return m.updateTerminalBellResult(msg), nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.inputSpinnerActive() {
			return m, cmd
		}
		return m, nil
	case tea.PasteMsg:
		return m.updateTeaPaste(msg), nil
	case composerPasteMsg:
		return m.updateComposerPaste(msg), nil
	}

	return m.updateInputAndViewport(msg)
}
