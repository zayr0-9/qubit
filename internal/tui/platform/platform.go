package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}

func ConfigDir() (string, error) {
	if override := os.Getenv("QUBIT_CONFIG_DIR"); override != "" {
		return override, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	name := "qubit"
	if os.PathSeparator == '\\' {
		name = "Qubit"
	}
	return filepath.Join(base, name), nil
}

type NotificationKind string

const NotificationKindRunComplete NotificationKind = "run_complete"

type NotificationPayload struct {
	Kind  NotificationKind
	Title string
	Body  string
}

type Notifier interface {
	Notify(NotificationPayload) error
}

type NoopNotifier struct{}

func (NoopNotifier) Notify(NotificationPayload) error { return nil }

func RunCompleteNotificationBody(sessionTitle string) string {
	sessionTitle = strings.TrimSpace(sessionTitle)
	if sessionTitle == "" {
		return "Agent response complete"
	}
	return fmt.Sprintf("Agent response complete: %s", sessionTitle)
}

func ShouldNotifyRunComplete(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "aborted", "abort", "cancelled", "canceled", "cancel", "error", "failed", "failure":
		return false
	default:
		return true
	}
}
