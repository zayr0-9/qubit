package tui

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
)

type model struct {
	width  int
	height int

	viewport         viewport.Model
	composer         composerModel
	spinner          spinner.Model
	inputCursorPulse int

	renderCache map[renderCacheKey]string

	messages              []chatMessage
	queuedMessages        []queuedMessage
	sessions              []sessionInfo
	pendingDeleteSession  sessionInfo
	apiKeys               []apiKeyInfo
	models                []modelInfo
	busy                  bool
	ready                 bool
	keyboardEnhanced      bool
	provider              string
	activeProvider        string
	activeKeyAlias        string
	model                 string
	maxContext            int
	reasoningLevel        reasoningLevel
	session               string
	title                 string
	status                string
	err                   string
	permissionMode        permissionMode
	cwdBlockEnabled       bool
	theme                 themeConfig
	autoNewSessionOnChat  bool
	lastRunStartedSession string
	activeRunID           string
	activeReasoningRunID  string
	activeReasoningIndex  int
	activeReasoningStart  int
	inputHistory          []string
	inputHistoryIndex     int
	inputHistoryActive    bool
	forkSelector          forkSelectorState
	messageEdit           messageEditState
	lastCodexUsage        *codexUsage

	mode                       uiMode
	previousMode               uiMode
	sessionCursor              int
	sessionSearchMode          bool
	sessionSearchQuery         string
	apiKeyCursor               int
	slashCursor                int
	fileMention                fileMentionState
	modal                      *modalState
	planClarification          planClarificationState
	keyEntry                   *keyEntryState
	themeEntry                 *themeEntryState
	forkTree                   forkTreeState
	mdEditor                   mdEditorState
	autoScroll                 bool
	toolHitboxes               []toolHitbox
	linkHitboxes               []linkHitbox
	todoOverlayExpanded        bool
	todoOverlayScroll          int
	todoOverlayKey             string
	todoOverlayMouseDownHeader bool
	chatTopY                   int
	transcriptSelection        transcriptSelectionState
	transcriptLines            []transcriptRenderLine
	transcriptContent          string

	toolCallRevealing          bool
	toolCallRevealMessageIndex int
	toolCallRevealVisibleRunes int

	streaming             bool
	streamingMessageIndex int
	streamingFullContent  string
	streamingVisibleRunes int
	streamingFinished     bool
	streamingFinishStatus string

	notifier notifier
	runtime  *runtimeClient
}
