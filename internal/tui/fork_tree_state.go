package tui

import "charm.land/bubbles/v2/viewport"

type forkTreeState struct {
	Loading              bool
	FocalSessionID       string
	Nodes                []forkTreeNode
	Selected             int
	Order                []int
	OffsetX              int
	OffsetY              int
	CanvasWidth          int
	CanvasHeight         int
	Preview              viewport.Model
	PreviewWidth         int
	PreviewHeight        int
	PreviewContentNodeID string
	PreviewContentWidth  int
}
