package tui

import "strings"

type chatListState struct {
	Width       int
	Height      int
	YOffset     int
	Items       []chatListItem
	Cache       map[chatListItemKey]chatListRenderedItem
	Visible     chatListVisibleRender
	TotalHeight int
}

type chatListItemKind int

const (
	chatItemMessage chatListItemKind = iota
	chatItemToolGroup
	chatItemReasoning
	chatItemView
)

type chatListItem struct {
	Kind       chatListItemKind
	StartIndex int
	EndIndex   int
	Key        chatListItemKey
}

type chatListItemKey struct {
	Segment messageRenderSegmentKey
}

type chatListRenderedItem struct {
	Key       chatListItemKey
	Text      string
	Lines     []string
	Plain     []transcriptRenderLine
	Links     []linkHitbox
	Tools     []toolHitbox
	Height    int
	LastIndex int
	Frozen    bool
}

type chatListVisibleRender struct {
	Content      string
	Lines        []transcriptRenderLine
	ToolHitboxes []toolHitbox
	LinkHitboxes []linkHitbox
	VisibleRows  []chatVisibleRow
	TotalHeight  int
}

type chatVisibleRow struct {
	ScreenY   int
	AbsoluteY int
	ItemIndex int
	LocalY    int
	Text      string
	Plain     transcriptRenderLine
}

func splitRenderedLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
