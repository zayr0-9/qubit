//go:build linux

package platform

import (
	"errors"
	"testing"
)

func TestNewLinuxNotifierFallsBackWhenNotifySendMissing(t *testing.T) {
	n := newLinuxNotifier(envWith("DBUS_SESSION_BUS_ADDRESS", "unix:path=/tmp/bus"), func(string) (string, error) {
		return "", errors.New("missing")
	})
	if _, ok := n.(NoopNotifier); !ok {
		t.Fatalf("notifier = %T, want NoopNotifier", n)
	}
}

func TestNewLinuxNotifierFallsBackWithoutDesktopSession(t *testing.T) {
	n := newLinuxNotifier(envWith("", ""), func(string) (string, error) {
		return "/usr/bin/notify-send", nil
	})
	if _, ok := n.(NoopNotifier); !ok {
		t.Fatalf("notifier = %T, want NoopNotifier", n)
	}
}

func TestNewLinuxNotifierUsesNotifySendWithDesktopSession(t *testing.T) {
	tests := []struct {
		name string
		env  string
	}{
		{name: "dbus", env: "DBUS_SESSION_BUS_ADDRESS"},
		{name: "wayland", env: "WAYLAND_DISPLAY"},
		{name: "x11", env: "DISPLAY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := newLinuxNotifier(envWith(tt.env, "present"), func(string) (string, error) {
				return "/usr/bin/notify-send", nil
			})
			ln, ok := n.(linuxNotifier)
			if !ok {
				t.Fatalf("notifier = %T, want linuxNotifier", n)
			}
			if ln.notifySendPath != "/usr/bin/notify-send" {
				t.Fatalf("notifySendPath = %q, want /usr/bin/notify-send", ln.notifySendPath)
			}
		})
	}
}

func TestSanitizeNotificationField(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		fallback string
		maxRunes int
		want     string
	}{
		{name: "trims", value: "  hello  ", maxRunes: 20, want: "hello"},
		{name: "fallback", value: " ", fallback: "Qubit", maxRunes: 20, want: "Qubit"},
		{name: "truncates", value: "abcdef", maxRunes: 4, want: "abc…"},
		{name: "single rune max", value: "abcdef", maxRunes: 1, want: "…"},
		{name: "zero max", value: "abcdef", maxRunes: 0, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeNotificationField(tt.value, tt.fallback, tt.maxRunes); got != tt.want {
				t.Fatalf("sanitizeNotificationField() = %q, want %q", got, tt.want)
			}
		})
	}
}

func envWith(name, value string) getenvFunc {
	return func(got string) string {
		if got == name {
			return value
		}
		return ""
	}
}
