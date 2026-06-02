package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type notificationKind string

const notificationKindRunComplete notificationKind = "run_complete"

type notificationPayload struct {
	Kind  notificationKind
	Title string
	Body  string
}

type notifier interface {
	Notify(notificationPayload) error
}

type noopNotifier struct{}

func (noopNotifier) Notify(notificationPayload) error { return nil }

func newPlatformNotifier() notifier {
	// Stub for future OS-specific implementations. Keep app lifecycle code
	// dependent only on the notifier interface so Windows/macOS/Linux backends can
	// be added behind this constructor without touching streaming/run logic.
	return noopNotifier{}
}

func notifyRunCompleteCmd(n notifier, sessionTitle string) tea.Cmd {
	if n == nil {
		return nil
	}
	payload := notificationPayload{
		Kind:  notificationKindRunComplete,
		Title: "Qubit",
		Body:  runCompleteNotificationBody(sessionTitle),
	}
	return func() tea.Msg {
		return notificationResultMsg{kind: payload.Kind, err: n.Notify(payload)}
	}
}

func runCompleteNotificationBody(sessionTitle string) string {
	sessionTitle = strings.TrimSpace(sessionTitle)
	if sessionTitle == "" {
		return "Agent response complete"
	}
	return fmt.Sprintf("Agent response complete: %s", sessionTitle)
}

func shouldNotifyRunComplete(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "aborted", "abort", "cancelled", "canceled", "cancel", "error", "failed", "failure":
		return false
	default:
		return true
	}
}
