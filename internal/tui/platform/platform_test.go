package platform

import "testing"

func TestRunCompleteNotificationBody(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{name: "empty", title: "", want: "Agent response complete"},
		{name: "whitespace", title: "  \t", want: "Agent response complete"},
		{name: "with title", title: "Demo chat", want: "Agent response complete: Demo chat"},
		{name: "trims title", title: "  Demo chat  ", want: "Agent response complete: Demo chat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RunCompleteNotificationBody(tt.title); got != tt.want {
				t.Fatalf("RunCompleteNotificationBody(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestShouldNotifyRunComplete(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{status: "", want: true},
		{status: "completed", want: true},
		{status: "ready", want: true},
		{status: " aborted ", want: false},
		{status: "cancelled", want: false},
		{status: "canceled", want: false},
		{status: "error", want: false},
		{status: "FAILED", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := ShouldNotifyRunComplete(tt.status); got != tt.want {
				t.Fatalf("ShouldNotifyRunComplete(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
