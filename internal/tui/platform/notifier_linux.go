//go:build linux

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	defaultNotificationTitle = "Qubit"
	maxNotificationTitleLen  = 120
	maxNotificationBodyLen   = 500
)

type linuxNotifier struct {
	notifySendPath string
}

type getenvFunc func(string) string
type lookPathFunc func(string) (string, error)

func NewNotifier() Notifier {
	return newLinuxNotifier(os.Getenv, exec.LookPath)
}

func newLinuxNotifier(getenv getenvFunc, lookPath lookPathFunc) Notifier {
	path, err := lookPath("notify-send")
	if err != nil || strings.TrimSpace(path) == "" {
		return NoopNotifier{}
	}
	if !hasDesktopNotificationSession(getenv) {
		return NoopNotifier{}
	}
	return linuxNotifier{notifySendPath: path}
}

func hasDesktopNotificationSession(getenv getenvFunc) bool {
	for _, name := range []string{"DBUS_SESSION_BUS_ADDRESS", "WAYLAND_DISPLAY", "DISPLAY"} {
		if strings.TrimSpace(getenv(name)) != "" {
			return true
		}
	}
	return false
}

func (n linuxNotifier) Notify(payload NotificationPayload) error {
	path := strings.TrimSpace(n.notifySendPath)
	if path == "" {
		return nil
	}

	title := sanitizeNotificationField(payload.Title, defaultNotificationTitle, maxNotificationTitleLen)
	body := sanitizeNotificationField(payload.Body, "", maxNotificationBodyLen)
	args := []string{
		"--app-name", defaultNotificationTitle,
		"--urgency", "normal",
		"--expire-time", "5000",
		title,
	}
	if body != "" {
		args = append(args, body)
	}
	cmd := exec.Command(path, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("send desktop notification: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func sanitizeNotificationField(value, fallback string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}
