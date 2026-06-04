package tui

type fileMentionEntry struct {
	Path  string
	Name  string
	IsDir bool
}

type fileMentionSelection struct {
	Display string
	Path    string
}
