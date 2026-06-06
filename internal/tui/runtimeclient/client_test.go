package runtimeclient

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qubit/graviton-cli/internal/tui/protocol"
)

type closeOnlyConn struct {
	closed  bool
	readCh  chan struct{}
	readErr error
}

func newCloseOnlyConn(readErr error) *closeOnlyConn {
	return &closeOnlyConn{readCh: make(chan struct{}), readErr: readErr}
}

func (c *closeOnlyConn) Read(_ []byte) (int, error) {
	<-c.readCh
	return 0, c.readErr
}
func (c *closeOnlyConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *closeOnlyConn) Close() error {
	if !c.closed {
		c.closed = true
		close(c.readCh)
	}
	return nil
}

func TestShutdownClosesConnectionOnly(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "runtime-server.lock")
	if err := os.WriteFile(lockPath, []byte("123\n127.0.0.1:20000\n"), 0644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	conn := newCloseOnlyConn(io.EOF)
	rt := &Client{conn: conn, stdin: conn, lockPath: lockPath, events: make(chan protocol.RuntimeEvent, 1), errs: make(chan error, 1)}

	rt.Shutdown()

	if !conn.closed {
		t.Fatal("connection was not closed")
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock removed on normal shutdown, want preserved: %v", err)
	}
}

func TestCleanupFailedStartRemovesLock(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "runtime-server.lock")
	if err := os.WriteFile(lockPath, []byte("123\n127.0.0.1:20000\n"), 0644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	rt := &Client{lockPath: lockPath, events: make(chan protocol.RuntimeEvent, 1), errs: make(chan error, 1)}

	rt.cleanupFailedStart()

	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock exists after failed-start cleanup: %v", err)
	}
}

func TestReadStdoutDisconnectHandling(t *testing.T) {
	rt := &Client{events: make(chan protocol.RuntimeEvent, 1), errs: make(chan error, 1), logPath: filepath.Join(t.TempDir(), "runtime.log")}
	unexpected := newCloseOnlyConn(io.EOF)
	rt.attachConn(unexpected, true)
	unexpected.Close()

	select {
	case err := <-rt.Errors():
		if !errors.Is(err, ErrDisconnected) {
			t.Fatalf("error = %v, want ErrDisconnected", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for disconnect error")
	}

	intentional := newCloseOnlyConn(io.EOF)
	rt.attachConn(intentional, true)
	rt.Shutdown()
	select {
	case err := <-rt.Errors():
		t.Fatalf("unexpected shutdown error: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSummarizeRuntimeEventCompactsSessionList(t *testing.T) {
	raw := []byte(`{"type":"session.list","id":"req-1","sessionId":"sess-1","sessionTitle":"Active","sessions":[{"id":"sess-1","title":"Active"},{"id":"sess-2","title":"Long old chat"}]}`)
	ev := protocol.RuntimeEvent{
		Type:         "session.list",
		ID:           "req-1",
		SessionID:    "sess-1",
		SessionTitle: "Active",
		Sessions: []protocol.SessionInfo{
			{ID: "sess-1", Title: "Active"},
			{ID: "sess-2", Title: "Long old chat"},
		},
	}

	got := summarizeRuntimeEvent(raw, ev)

	if !strings.Contains(got, "session.list") || !strings.Contains(got, "sessions=2") {
		t.Fatalf("summary = %q, want compact session count", got)
	}
	if strings.Contains(got, "Long old chat") || strings.Contains(got, `"sessions"`) {
		t.Fatalf("summary = %q, want session payload omitted", got)
	}
}

func TestReadStdoutHandlesLargeRuntimeEventLine(t *testing.T) {
	large := strings.Repeat("x", 2*1024*1024)
	rt := &Client{events: make(chan protocol.RuntimeEvent, 1), errs: make(chan error, 1), logPath: filepath.Join(t.TempDir(), "runtime.log")}
	rt.readStdout(strings.NewReader(`{"type":"assistant","content":"`+large+`"}`+"\n"), 0)

	select {
	case ev := <-rt.Events():
		if ev.Type != "assistant" || len(ev.Content) != len(large) {
			t.Fatalf("event type=%q content len=%d, want assistant len=%d", ev.Type, len(ev.Content), len(large))
		}
	default:
		t.Fatal("large runtime event was not delivered")
	}
}
