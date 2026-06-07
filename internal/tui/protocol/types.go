package protocol

type ToolCallUI struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Step         int            `json:"step"`
	Status       string         `json:"status"`
	Args         map[string]any `json:"args,omitempty"`
	Result       map[string]any `json:"result,omitempty"`
	ContextChars int            `json:"contextChars,omitempty"`
	StartedAt    string         `json:"startedAt,omitempty"`
	FinishedAt   string         `json:"finishedAt,omitempty"`
	DurationMs   int            `json:"durationMs,omitempty"`
}

type ToolGroup struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Step     int          `json:"step"`
	Calls    []ToolCallUI `json:"calls"`
	Expanded bool         `json:"expanded,omitempty"`
}

type ChatMessage struct {
	Role             string      `json:"role"`
	Content          string      `json:"content"`
	ReasoningContent string      `json:"reasoningContent,omitempty"`
	ViewType         string      `json:"viewType,omitempty"`
	Title            string      `json:"title,omitempty"`
	Path             string      `json:"path,omitempty"`
	URL              string      `json:"url,omitempty"`
	MimeType         string      `json:"mimeType,omitempty"`
	SizeBytes        int         `json:"sizeBytes,omitempty"`
	ToolGroup        *ToolGroup  `json:"toolGroup,omitempty"`
	CodexUsage       *CodexUsage `json:"codexUsage,omitempty"`
	LocalOnly        bool        `json:"localOnly,omitempty"`
	MessageKind      string      `json:"messageKind,omitempty"`
	Expanded         bool        `json:"expanded,omitempty"`
}

type ForkTreeLineageMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ForkTreeMessageNode struct {
	ID           string `json:"id"`
	ParentID     string `json:"parentId,omitempty"`
	SessionID    string `json:"sessionId"`
	SessionTitle string `json:"sessionTitle,omitempty"`
	Role         string `json:"role"`
	Content      string `json:"content"`
	MessageIndex int    `json:"messageIndex,omitempty"`
	Continued    bool   `json:"continued,omitempty"`
}

type SessionInfo struct {
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
	FavouritedAt           string `json:"favouritedAt,omitempty"`
	Hidden                 bool   `json:"hidden,omitempty"`
	Kind                   string `json:"kind,omitempty"`
}

type ApiKeyInfo struct {
	Provider  string `json:"provider"`
	Alias     string `json:"alias"`
	Source    string `json:"source"`
	Active    bool   `json:"active"`
	Masked    string `json:"masked"`
	Readonly  bool   `json:"readonly"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	MaxContext  int    `json:"maxContext,omitempty"`
}

type McpCatalogEntry struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Transport    string   `json:"transport"`
	URL          string   `json:"url"`
	AuthTypes    []string `json:"authTypes,omitempty"`
	DefaultAuth  string   `json:"defaultAuthType,omitempty"`
	DocsURL      string   `json:"docsUrl,omitempty"`
	RepoURL      string   `json:"repoUrl,omitempty"`
	ToolsSummary string   `json:"toolsSummary,omitempty"`
	Caveat       string   `json:"caveat,omitempty"`
	Safety       string   `json:"safety,omitempty"`
}

type McpServerInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	Transport     string `json:"transport"`
	URL           string `json:"url,omitempty"`
	Command       string `json:"command,omitempty"`
	AuthType      string `json:"authType,omitempty"`
	AuthStatus    string `json:"authStatus,omitempty"`
	Status        string `json:"status,omitempty"`
	StatusMessage string `json:"statusMessage,omitempty"`
	ToolCount     int    `json:"toolCount,omitempty"`
	CatalogID     string `json:"catalogId,omitempty"`
	Caveat        string `json:"caveat,omitempty"`
}

type McpToolInfo struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type PlanClarificationOption struct {
	ID          string `json:"id,omitempty"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Manual      bool   `json:"manual,omitempty"`
}

type PlanClarificationQuestion struct {
	ID          string                    `json:"id,omitempty"`
	Question    string                    `json:"question,omitempty"`
	Description string                    `json:"description,omitempty"`
	Options     []PlanClarificationOption `json:"options,omitempty"`
}

type PlanClarificationAnswer struct {
	QuestionID          string `json:"questionId,omitempty"`
	Question            string `json:"question,omitempty"`
	SelectedOptionID    string `json:"selectedOptionId,omitempty"`
	SelectedOptionLabel string `json:"selectedOptionLabel,omitempty"`
	Manual              bool   `json:"manual,omitempty"`
	Answer              string `json:"answer,omitempty"`
}

type MdFileInfo struct {
	Section    string `json:"section"`
	Name       string `json:"name"`
	Title      string `json:"title,omitempty"`
	Path       string `json:"path"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
	SizeBytes  int    `json:"sizeBytes,omitempty"`
}

type ForkTreeNode struct {
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
	LineageMessages        []ForkTreeLineageMessage `json:"lineageMessages,omitempty"`
	TranscriptMessages     []ChatMessage            `json:"transcriptMessages,omitempty"`
	MessageNodes           []ForkTreeMessageNode    `json:"messageNodes,omitempty"`
	MessageCount           int                      `json:"messageCount,omitempty"`

	X        int   `json:"-"`
	Y        int   `json:"-"`
	Parent   int   `json:"-"`
	Children []int `json:"-"`
}

type RuntimeEvent struct {
	Type             string                      `json:"type"`
	ID               string                      `json:"id,omitempty"`
	ClientID         string                      `json:"clientId,omitempty"`
	SessionID        string                      `json:"sessionId,omitempty"`
	RunID            string                      `json:"runId,omitempty"`
	SessionTitle     string                      `json:"sessionTitle,omitempty"`
	Session          *SessionInfo                `json:"session,omitempty"`
	Sessions         []SessionInfo               `json:"sessions,omitempty"`
	Messages         []ChatMessage               `json:"messages,omitempty"`
	Questions        []PlanClarificationQuestion `json:"questions,omitempty"`
	Answers          []PlanClarificationAnswer   `json:"answers,omitempty"`
	Provider         string                      `json:"provider,omitempty"`
	Providers        []ModelInfo                 `json:"providers,omitempty"`
	ActiveProvider   string                      `json:"activeProvider,omitempty"`
	ActiveKeyAlias   string                      `json:"activeKeyAlias,omitempty"`
	Model            string                      `json:"model,omitempty"`
	SubagentProvider string                      `json:"subagentProvider,omitempty"`
	SubagentModel    string                      `json:"subagentModel,omitempty"`
	Keys             []ApiKeyInfo                `json:"keys,omitempty"`
	Models           []ModelInfo                 `json:"models,omitempty"`
	McpServers       []McpServerInfo             `json:"servers,omitempty"`
	McpCatalog       []McpCatalogEntry           `json:"catalog,omitempty"`
	McpTools         []McpToolInfo               `json:"tools,omitempty"`
	Success          bool                        `json:"success,omitempty"`
	ForkTreeNodes    []ForkTreeNode              `json:"nodes,omitempty"`
	Files            []MdFileInfo                `json:"files,omitempty"`
	File             *MdFileInfo                 `json:"file,omitempty"`
	StoragePath      string                      `json:"storagePath,omitempty"`
	IndexPath        string                      `json:"indexPath,omitempty"`
	WorkspaceCwd     string                      `json:"workspaceCwd,omitempty"`
	AuthURL          string                      `json:"authUrl,omitempty"`
	LocalPort        int                         `json:"localPort,omitempty"`
	AccountEmail     string                      `json:"accountEmail,omitempty"`
	AccountID        string                      `json:"accountId,omitempty"`
	Storage          string                      `json:"storage,omitempty"`
	Active           bool                        `json:"active,omitempty"`
	Status           string                      `json:"status,omitempty"`
	Content          string                      `json:"content,omitempty"`
	Name             string                      `json:"name,omitempty"`
	Path             string                      `json:"path,omitempty"`
	URL              string                      `json:"url,omitempty"`
	MimeType         string                      `json:"mimeType,omitempty"`
	SizeBytes        int                         `json:"sizeBytes,omitempty"`
	Cwd              string                      `json:"cwd,omitempty"`
	ReasoningContent string                      `json:"reasoningContent,omitempty"`
	ReasoningLevel   string                      `json:"reasoningLevel,omitempty"`
	MaxContext       int                         `json:"maxContext,omitempty"`
	Step             int                         `json:"step,omitempty"`
	ToolCallID       string                      `json:"toolCallId,omitempty"`
	ToolName         string                      `json:"toolName,omitempty"`
	Args             map[string]any              `json:"args,omitempty"`
	Result           map[string]any              `json:"result,omitempty"`
	ContextChars     int                         `json:"contextChars,omitempty"`
	CodexUsage       *CodexUsage                 `json:"codexUsage,omitempty"`
	StartedAt        string                      `json:"startedAt,omitempty"`
	FinishedAt       string                      `json:"finishedAt,omitempty"`
	DurationMs       int                         `json:"durationMs,omitempty"`
	Description      string                      `json:"description,omitempty"`
	Metadata         map[string]any              `json:"metadata,omitempty"`
	InputSchema      map[string]any              `json:"inputSchema,omitempty"`
	Error            string                      `json:"error,omitempty"`
}

type CodexUsage struct {
	InputTokens  int    `json:"inputTokens,omitempty"`
	CachedTokens int    `json:"cachedTokens,omitempty"`
	OutputTokens int    `json:"outputTokens,omitempty"`
	TotalTokens  int    `json:"totalTokens,omitempty"`
	CallID       string `json:"callId,omitempty"`
	ResponseID   string `json:"responseId,omitempty"`
	Model        string `json:"model,omitempty"`
	Status       string `json:"status,omitempty"`
	DurationMs   int    `json:"durationMs,omitempty"`
	FinishedAt   string `json:"finishedAt,omitempty"`
}
