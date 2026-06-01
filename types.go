package main

import (
	"io"
	"os/exec"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
)

type chatMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolGroup *toolGroup `json:"toolGroup,omitempty"`
}

type renderCacheKey struct {
	Role    string
	Content string
	Width   int
}

type sessionInfo struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	MessageCount int    `json:"messageCount"`
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

type keyEntryStep int

const (
	keyEntryProvider keyEntryStep = iota
	keyEntryAlias
	keyEntrySecret
)

type keyEntryState struct {
	Step     keyEntryStep
	Provider composerModel
	Alias    composerModel
	Secret   composerModel
}

type uiMode int

const (
	modeChat uiMode = iota
	modeSessionPicker
	modeKeyPicker
	modeKeyEntry
	modeModal
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

type modalState struct {
	ID          string
	Kind        modalKind
	Title       string
	Description string
	Fields      []modalField
	Actions     []modalAction
	Cursor      int
	Payload     map[string]any
}

type slashCommand struct {
	Name        string
	Usage       string
	Description string
	NeedsArg    bool
}

type runtimeEvent struct {
	Type             string         `json:"type"`
	ID               string         `json:"id,omitempty"`
	SessionID        string         `json:"sessionId,omitempty"`
	SessionTitle     string         `json:"sessionTitle,omitempty"`
	Session          *sessionInfo   `json:"session,omitempty"`
	Sessions         []sessionInfo  `json:"sessions,omitempty"`
	Messages         []chatMessage  `json:"messages,omitempty"`
	Provider         string         `json:"provider,omitempty"`
	ActiveProvider   string         `json:"activeProvider,omitempty"`
	ActiveKeyAlias   string         `json:"activeKeyAlias,omitempty"`
	Model            string         `json:"model,omitempty"`
	Keys             []apiKeyInfo   `json:"keys,omitempty"`
	StoragePath      string         `json:"storagePath,omitempty"`
	IndexPath        string         `json:"indexPath,omitempty"`
	WorkspaceCwd     string         `json:"workspaceCwd,omitempty"`
	Status           string         `json:"status,omitempty"`
	Content          string         `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoningContent,omitempty"`
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

type runtimeClient struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	events    chan runtimeEvent
	errs      chan error
	appRoot   string
	launchCwd string
	logPath   string
}

type model struct {
	width  int
	height int

	viewport viewport.Model
	composer composerModel
	spinner  spinner.Model

	renderCache map[renderCacheKey]string

	messages         []chatMessage
	sessions         []sessionInfo
	apiKeys          []apiKeyInfo
	busy             bool
	ready            bool
	keyboardEnhanced bool
	provider         string
	activeProvider   string
	activeKeyAlias   string
	model            string
	session          string
	title            string
	status           string
	err              string
	permissionMode   permissionMode

	mode          uiMode
	previousMode  uiMode
	sessionCursor int
	apiKeyCursor  int
	slashCursor   int
	modal         *modalState
	keyEntry      *keyEntryState
	autoScroll    bool
	toolHitboxes  []toolHitbox
	chatTopY      int

	streaming             bool
	streamingMessageIndex int
	streamingFullContent  string
	streamingVisibleRunes int
	streamingFinished     bool
	streamingFinishStatus string

	runtime *runtimeClient
}
