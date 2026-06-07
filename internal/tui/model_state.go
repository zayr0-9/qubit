package tui

type reasoningLevel string

const (
	reasoningLevelNone   reasoningLevel = "none"
	reasoningLevelLow    reasoningLevel = "low"
	reasoningLevelMedium reasoningLevel = "medium"
	reasoningLevelHigh   reasoningLevel = "high"
)

type fileMentionState struct {
	Entries    []fileMentionEntry
	Cursor     int
	IndexedCwd string
	Err        string
	Selections []fileMentionSelection
}

type uiMode int

const (
	modeChat uiMode = iota
	modeSessionPicker
	modeKeyPicker
	modeKeyEntry
	modeThemeEntry
	modeModal
	modeForkTree
	modeMdEditor
	modeMcpManager
	modeMcpAddEntry
	modeMcpSecretEntry
)

type permissionMode string

const (
	permissionModeAsk         permissionMode = "ask"
	permissionModeAlwaysAllow permissionMode = "always_allow"
	permissionModeAllowAll    permissionMode = "allow_all"
)
