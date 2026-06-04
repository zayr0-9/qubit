package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/qubit/graviton-cli/internal/tui/platform"
)

func openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if err := platform.OpenBrowser(url); err != nil {
			return runtimeErrMsg{err: err}
		}
		return nil
	}
}

func qubitConfigDir() (string, error) { return platform.ConfigDir() }

type notificationKind = platform.NotificationKind

const notificationKindRunComplete = platform.NotificationKindRunComplete

type notificationPayload = platform.NotificationPayload
type notifier = platform.Notifier

func newPlatformNotifier() notifier { return platform.NewNotifier() }

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
	return platform.RunCompleteNotificationBody(sessionTitle)
}

func shouldNotifyRunComplete(status string) bool { return platform.ShouldNotifyRunComplete(status) }

var _ = fmt.Sprintf
