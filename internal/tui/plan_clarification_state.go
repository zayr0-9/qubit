package tui

type planClarificationState struct {
	Active       bool
	RequestID    string
	SessionID    string
	RunID        string
	Step         int
	ToolCallID   string
	Questions    []planClarificationQuestion
	Answers      []planClarificationAnswer
	Index        int
	OptionCursor int
	Manual       composerModel
}
