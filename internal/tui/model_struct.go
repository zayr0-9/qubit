package tui

import (
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
)

type model struct {
	width  int
	height int

	viewport         viewport.Model
	chatList         chatListState
	composer         composerModel
	spinner          spinner.Model
	inputCursorPulse int

	renderCache              map[renderCacheKey]string
	markdownRenderers        markdownRendererCache
	messageRenderSegments    []messageRenderSegment
	streamingMarkdownCache   streamingMarkdownCache
	streamingTranscriptCache streamingTranscriptCache

	messages              []chatMessage
	queuedMessages        []queuedMessage
	sessions              []sessionInfo
	pendingDeleteSession  sessionInfo
	apiKeys               []apiKeyInfo
	models                []modelInfo
	subagentModels        []modelInfo
	subagentProviders     []modelInfo
	mcpServers            []mcpServerInfo
	mcpCatalog            []mcpCatalogEntry
	busy                  bool
	ready                 bool
	runtimeConnected      bool
	keyboardEnhanced      bool
	provider              string
	activeProvider        string
	activeKeyAlias        string
	model                 string
	maxContext            int
	reasoningLevel        reasoningLevel
	subagentProvider      string
	subagentModel         string
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
	activeRunStartedAt    time.Time
	activeReasoningRunID  string
	activeReasoningIndex  int
	activeReasoningStart  int
	inputHistory          []string
	inputHistoryIndex     int
	inputHistoryActive    bool
	forkSelector          forkSelectorState
	messageEdit           messageEditState
	lastCodexUsage        *codexUsage
	transcriptLoadRunID   string
	transcriptLoadSession string
	pendingCompactInput   string
	compacting            bool
	lastCompactedSource   string

	mode                       uiMode
	previousMode               uiMode
	sessionCursor              int
	sessionSearchMode          bool
	sessionSearchQuery         string
	apiKeyCursor               int
	mcpCursor                  int
	mcpCatalogCursor           int
	slashCursor                int
	fileMention                fileMentionState
	modal                      *modalState
	planClarification          planClarificationState
	keyEntry                   *keyEntryState
	themeEntry                 *themeEntryState
	mcpAddEntry                *mcpAddEntryState
	mcpSecretEntry             *mcpSecretEntryState
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
