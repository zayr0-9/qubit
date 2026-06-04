package runtimeclient

import (
	"errors"
	"io"
	"os"
	"path/filepath"
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
