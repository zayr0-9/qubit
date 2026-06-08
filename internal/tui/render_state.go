package tui

import "github.com/charmbracelet/glamour"

type renderCacheKey struct {
	Role             string
	Content          string
	ReasoningContent string
	Width            int
}

type markdownRendererCacheKey struct {
	Width int
	Theme string
}

type messageRenderSegmentKey struct {
	Index                    int
	Role                     string
	Content                  string
	ReasoningContent         string
	ViewType                 string
	Title                    string
	Path                     string
	MessageKind              string
	Expanded                 bool
	IsLastMessage            bool
	ToolGroupID              string
	ToolGroupLabel           string
	ToolGroupExpanded        bool
	ToolCallRevealVisible    int
	Width                    int
	DisplayNumber            int
	UserMessageBg            string
	PreviousRole             string
	PreviousContent          string
	PreviousReasoningContent string
}

type messageRenderSegment struct {
	Key       messageRenderSegmentKey
	Text      string
	Tools     []toolHitbox
	LineCount int
	LastIndex int
}

type streamingMarkdownCache struct {
	Content  string
	Width    int
	Rendered string
}

type markdownRendererCache map[markdownRendererCacheKey]*glamour.TermRenderer

type streamingTranscriptCache struct {
	MessageIndex    int
	Width           int
	Prefix          string
	PrefixTools     []toolHitbox
	PrefixLineCount int
}
