package tui

type mcpAddEntryKind string

const (
	mcpAddRemote mcpAddEntryKind = "remote"
	mcpAddStdio  mcpAddEntryKind = "stdio"
)

type mcpAddEntryStep int

const (
	mcpAddName mcpAddEntryStep = iota
	mcpAddURL
	mcpAddCommand
	mcpAddArgs
)

type mcpAddEntryState struct {
	Kind    mcpAddEntryKind
	Step    mcpAddEntryStep
	Name    composerModel
	URL     composerModel
	Command composerModel
	Args    composerModel
	Err     string
}

type mcpSecretEntryState struct {
	ServerID string
	Name     string
	Secret   composerModel
}
