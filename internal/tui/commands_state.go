package tui

type slashCommand struct {
	Name          string
	Usage         string
	Description   string
	NeedsArg      bool
	OpensOnSelect bool
}
