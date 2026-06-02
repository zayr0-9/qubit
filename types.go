package main

import (
	"io"
	"os/exec"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
)

type chatMessage struct {
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoningContent,omitempty"`
	ViewType         string     `json:"viewType,omitempty"`
	Title            string     `json:"title,omitempty"`
	Path             string     `json:"path,omitempty"`
	ToolGroup        *toolGroup `json:"toolGroup,omitempty"`
}

type forkTreeLineageMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type forkTreeMessageNode struct {
	ID           string `json:"id"`
	ParentID     string `json:"parentId,omitempty"`
	SessionID    string `json:"sessionId"`
	SessionTitle string `json:"sessionTitle,omitempty"`
	Role         string `json:"role"`
	Content      string `json:"content"`
	MessageIndex int    `json:"messageIndex,omitempty"`
	Continued    bool   `json:"continued,omitempty"`
}

type renderCacheKey struct {
	Role             string
	Content          string
	ReasoningContent string
	Width            int
}

type sessionInfo struct {
	ID                     string `json:"id"`
	Title                  string `json:"title"`
	CreatedAt              string `json:"createdAt"`
	UpdatedAt              string `json:"updatedAt"`
	Provider               string `json:"provider"`
	Model                  string `json:"model"`
	MessageCount           int    `json:"messageCount"`
	ForkedFromSessionID    string `json:"forkedFromSessionId,omitempty"`
	ForkedFromMessageIndex int    `json:"forkedFromMessageIndex,omitempty"`
	ForkedAt               string `json:"forkedAt,omitempty"`
}

type apiKeyInfo struct {
	Provider  string `json:"provider"`
	Alias     string `json:"alias"`
	Source    string `json:"source"`
	Active    bool   `json:"active"`
	Masked    string `json:"masked"`
	Readonly  bool   `json:"readonly"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type modelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	MaxContext  int    `json:"maxContext,omitempty"`
}

type apiKeyProviderOption struct {
	ID          string
	Label       string
	Description string
}

type keyEntryStep int

const (
	keyEntryProvider keyEntryStep = iota
	keyEntryAlias
	keyEntrySecret
)

type keyEntryState struct {
	Step           keyEntryStep
	ProviderCursor int
	Providers      []apiKeyProviderOption
	Provider       composerModel
	Alias          composerModel
	Secret         composerModel
}

type themeEntryStep int

const (
	themeEntryPresets themeEntryStep = iota
	themeEntryBackground
	themeEntryText
)

type themeEntryState struct {
	Step       themeEntryStep
	Preset     int
	Background composerModel
	Text       composerModel
	Err        string
}

type forkPoint struct {
	Number           int
	MessageIndex     int
	EditMessageIndex int
	Content          string
}

type forkSelectorState struct {
	Active bool
	Points []forkPoint
	Cursor int
}

type messageEditState struct {
	Active       bool
	MessageIndex int
	Original     string
}

type fileMentionEntry struct {
	Path  string
	Name  string
	IsDir bool
}

type fileMentionSelection struct {
	Display string
	Path    string
}

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
)

type permissionMode string

const (
	permissionModeAsk         permissionMode = "ask"
	permissionModeAlwaysAllow permissionMode = "always_allow"
)

type modalKind string

const (
	modalKindPermission modalKind = "permission"
	modalKindConfirm    modalKind = "confirm"
	modalKindQuestion   modalKind = "question"
	modalKindCustom     modalKind = "custom"
)

type modalAction struct {
	ID      string
	Label   string
	Style   string
	Default bool
}

type modalField struct {
	Label string
	Value string
}
type modalOption struct {
	ID          string
	Label       string
	Description string
	Active      bool
}

type modalState struct {
	ID           string
	Kind         modalKind
	Title        string
	Description  string
	Fields       []modalField
	Options      []modalOption
	OptionCursor int
	Actions      []modalAction
	Cursor       int
	Payload      map[string]any
}

type slashCommand struct {
	Name          string
	Usage         string
	Description   string
	NeedsArg      bool
	OpensOnSelect bool
}

type forkTreeNode struct {
	ID                     string                   `json:"id"`
	SessionID              string                   `json:"sessionId"`
	SessionTitle           string                   `json:"sessionTitle"`
	ParentSessionID        string                   `json:"parentSessionId,omitempty"`
	ParentNodeID           string                   `json:"parentNodeId,omitempty"`
	ForkedFromMessageIndex int                      `json:"forkedFromMessageIndex,omitempty"`
	ForkedAt               string                   `json:"forkedAt,omitempty"`
	CreatedAt              string                   `json:"createdAt,omitempty"`
	UpdatedAt              string                   `json:"updatedAt,omitempty"`
	MessageRole            string                   `json:"messageRole,omitempty"`
	MessageContent         string                   `json:"messageContent,omitempty"`
	AssistantRole          string                   `json:"assistantRole,omitempty"`
	AssistantContent       string                   `json:"assistantContent,omitempty"`
	LineageMessages        []forkTreeLineageMessage `json:"lineageMessages,omitempty"`
	MessageNodes           []forkTreeMessageNode    `json:"messageNodes,omitempty"`
	MessageCount           int                      `json:"messageCount,omitempty"`

	X        int   `json:"-"`
	Y        int   `json:"-"`
	Parent   int   `json:"-"`
	Children []int `json:"-"`
}

type forkTreeState struct {
	Loading       bool
	Nodes         []forkTreeNode
	Selected      int
	Order         []int
	OffsetX       int
	OffsetY       int
	CanvasWidth   int
	CanvasHeight  int
	Preview       viewport.Model
	PreviewWidth  int
	PreviewHeight int
}

type runtimeEvent struct {
	Type             string         `json:"type"`
	ID               string         `json:"id,omitempty"`
	SessionID        string         `json:"sessionId,omitempty"`
	RunID            string         `json:"runId,omitempty"`
	SessionTitle     string         `json:"sessionTitle,omitempty"`
	Session          *sessionInfo   `json:"session,omitempty"`
	Sessions         []sessionInfo  `json:"sessions,omitempty"`
	Messages         []chatMessage  `json:"messages,omitempty"`
	Provider         string         `json:"provider,omitempty"`
	ActiveProvider   string         `json:"activeProvider,omitempty"`
	ActiveKeyAlias   string         `json:"activeKeyAlias,omitempty"`
	Model            string         `json:"model,omitempty"`
	Keys             []apiKeyInfo   `json:"keys,omitempty"`
	Models           []modelInfo    `json:"models,omitempty"`
	ForkTreeNodes    []forkTreeNode `json:"nodes,omitempty"`
	StoragePath      string         `json:"storagePath,omitempty"`
	IndexPath        string         `json:"indexPath,omitempty"`
	WorkspaceCwd     string         `json:"workspaceCwd,omitempty"`
	AuthURL          string         `json:"authUrl,omitempty"`
	LocalPort        int            `json:"localPort,omitempty"`
	AccountEmail     string         `json:"accountEmail,omitempty"`
	AccountID        string         `json:"accountId,omitempty"`
	Storage          string         `json:"storage,omitempty"`
	Active           bool           `json:"active,omitempty"`
	Status           string         `json:"status,omitempty"`
	Content          string         `json:"content,omitempty"`
	Name             string         `json:"name,omitempty"`
	Path             string         `json:"path,omitempty"`
	Cwd              string         `json:"cwd,omitempty"`
	ReasoningContent string         `json:"reasoningContent,omitempty"`
	ReasoningLevel   string         `json:"reasoningLevel,omitempty"`
	MaxContext       int            `json:"maxContext,omitempty"`
	Step             int            `json:"step,omitempty"`
	ToolCallID       string         `json:"toolCallId,omitempty"`
	ToolName         string         `json:"toolName,omitempty"`
	Args             map[string]any `json:"args,omitempty"`
	Result           map[string]any `json:"result,omitempty"`
	StartedAt        string         `json:"startedAt,omitempty"`
	FinishedAt       string         `json:"finishedAt,omitempty"`
	DurationMs       int            `json:"durationMs,omitempty"`
	Description      string         `json:"description,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	InputSchema      map[string]any `json:"inputSchema,omitempty"`
	Error            string         `json:"error,omitempty"`
}

type runtimeMsg runtimeEvent
type runtimeErrMsg struct{ err error }
type sendDoneMsg struct{ err error }
type terminalSetupResultMsg terminalSetupResult
type fakeStreamTickMsg struct{}
type toolCallRevealTickMsg struct{}
type inputCursorPulseMsg struct{}
type notificationResultMsg struct {
	kind notificationKind
	err  error
}

type runtimeClient struct {
	cmd       *exec.Cmd
	conn      io.ReadWriteCloser
	stdin     io.WriteCloser
	events    chan runtimeEvent
	errs      chan error
	appRoot   string
	launchCwd string
	qubitDir  string
	logPath   string
	lockPath  string
	attached  bool
}

type model struct {
	width  int
	height int

	viewport         viewport.Model
	composer         composerModel
	spinner          spinner.Model
	inputCursorPulse int

	renderCache map[renderCacheKey]string

	messages              []chatMessage
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
	theme                 themeConfig
	autoNewSessionOnChat  bool
	lastRunStartedSession string
	activeRunID           string
	inputHistory          []string
	inputHistoryIndex     int
	inputHistoryActive    bool
	forkSelector          forkSelectorState
	messageEdit           messageEditState

	mode          uiMode
	previousMode  uiMode
	sessionCursor int
	apiKeyCursor  int
	slashCursor   int
	fileMention   fileMentionState
	modal         *modalState
	keyEntry      *keyEntryState
	themeEntry    *themeEntryState
	forkTree      forkTreeState
	autoScroll    bool
	toolHitboxes  []toolHitbox
	chatTopY      int

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
