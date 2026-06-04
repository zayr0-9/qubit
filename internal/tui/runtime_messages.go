package tui

type runtimeMsg runtimeEvent
type runtimeErrMsg struct{ err error }
type sendDoneMsg struct{ err error }
type terminalSetupResultMsg terminalSetupResult
type fakeStreamTickMsg struct{}
type toolCallRevealTickMsg struct{}
type inputCursorPulseMsg struct{}
type notificationResultMsg struct {
	kind notificationKind
	err  error
}
