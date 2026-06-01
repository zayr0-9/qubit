package main

import (
	"io"
	"os/exec"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
)

type chatMessage struct {
	Role    string
	Content string
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

type uiMode int

const (
	modeChat uiMode = iota
	modeSessionPicker
)

type slashCommand struct {
	Name        string
	Usage       string
	Description string
	NeedsArg    bool
}

type runtimeEvent struct {
	Type             string        `json:"type"`
	ID               string        `json:"id,omitempty"`
	SessionID        string        `json:"sessionId,omitempty"`
	SessionTitle     string        `json:"sessionTitle,omitempty"`
	Session          *sessionInfo  `json:"session,omitempty"`
	Sessions         []sessionInfo `json:"sessions,omitempty"`
	Provider         string        `json:"provider,omitempty"`
	Model            string        `json:"model,omitempty"`
	StoragePath      string        `json:"storagePath,omitempty"`
	IndexPath        string        `json:"indexPath,omitempty"`
	Status           string        `json:"status,omitempty"`
	Content          string        `json:"content,omitempty"`
	ReasoningContent string        `json:"reasoningContent,omitempty"`
	Error            string        `json:"error,omitempty"`
}

type runtimeMsg runtimeEvent
type runtimeErrMsg struct{ err error }
type sendDoneMsg struct{ err error }

type runtimeClient struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	events  chan runtimeEvent
	errs    chan error
	appRoot string
	logPath string
}

type model struct {
	width  int
	height int

	viewport viewport.Model
	input    textinput.Model
	spinner  spinner.Model

	messages []chatMessage
	sessions []sessionInfo
	busy     bool
	ready    bool
	provider string
	model    string
	session  string
	title    string
	status   string
	err      string

	mode          uiMode
	sessionCursor int
	slashCursor   int

	runtime *runtimeClient
}
