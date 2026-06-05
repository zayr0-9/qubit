package runtimeclient

import (
	"bufio"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/qubit/graviton-cli/internal/tui/protocol"
)

var ErrDisconnected = errors.New("runtime disconnected")

type Client struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	conn    io.ReadWriteCloser
	stdin   io.WriteCloser
	events  chan protocol.RuntimeEvent
	errs    chan error
	closed  bool
	connSeq int

	appRoot   string
	launchCwd string
	qubitDir  string
	logPath   string
	lockPath  string
	attached  bool
}

type clientContext struct {
	node        string
	launchCwd   string
	appRoot     string
	qubitDir    string
	logPath     string
	runtimePath string
	serverAddr  string
	lockPath    string
}

func Start() (*Client, error) {
	ctx, err := newClientContext()
	if err != nil {
		return nil, err
	}
	rt := newClient(ctx)
	if err := rt.attachOrStart(ctx); err != nil {
		rt.cleanupFailedStart()
		return nil, err
	}
	return rt, nil
}

func newClient(ctx clientContext) *Client {
	return &Client{
		events:    make(chan protocol.RuntimeEvent, 32),
		errs:      make(chan error, 4),
		appRoot:   ctx.appRoot,
		launchCwd: ctx.launchCwd,
		qubitDir:  ctx.qubitDir,
		logPath:   ctx.logPath,
		lockPath:  ctx.lockPath,
	}
}

func newClientContext() (clientContext, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return clientContext{}, err
	}
	launchCwd, err := os.Getwd()
	if err != nil {
		return clientContext{}, fmt.Errorf("get launch cwd: %w", err)
	}
	appRoot, err := findAppRoot()
	if err != nil {
		return clientContext{}, err
	}
	qubitDir := filepath.Join(launchCwd, ".qubit")
	if err := os.MkdirAll(qubitDir, 0755); err != nil {
		return clientContext{}, fmt.Errorf("create project .qubit directory: %w", err)
	}
	return makeClientContext(node, launchCwd, appRoot, qubitDir)
}

func makeClientContext(node, launchCwd, appRoot, qubitDir string) (clientContext, error) {
	logPath := filepath.Join(qubitDir, "runtime.log")
	runtimePath := filepath.Join(appRoot, "dist", "runtime.js")
	if _, err := os.Stat(runtimePath); err != nil {
		return clientContext{}, fmt.Errorf("runtime not built at %s; run pnpm run build:runtime", runtimePath)
	}
	serverAddr := runtimeServerAddress(qubitDir)
	lockPath := filepath.Join(qubitDir, "runtime-server.lock")
	return clientContext{
		node:        node,
		launchCwd:   launchCwd,
		appRoot:     appRoot,
		qubitDir:    qubitDir,
		logPath:     logPath,
		runtimePath: runtimePath,
		serverAddr:  serverAddr,
		lockPath:    lockPath,
	}, nil
}

func (r *Client) context() (clientContext, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return clientContext{}, err
	}
	if r.launchCwd == "" || r.appRoot == "" || r.qubitDir == "" {
		return newClientContext()
	}
	if err := os.MkdirAll(r.qubitDir, 0755); err != nil {
		return clientContext{}, fmt.Errorf("create project .qubit directory: %w", err)
	}
	return makeClientContext(node, r.launchCwd, r.appRoot, r.qubitDir)
}

func (r *Client) attachOrStart(ctx clientContext) error {
	if err := r.connectExisting(ctx.serverAddr, 150*time.Millisecond); err == nil {
		return nil
	}
	return r.startServer(ctx)
}

func (r *Client) connectExisting(address string, timeout time.Duration) error {
	conn, err := connectRuntimeServer(address, timeout)
	if err != nil {
		return err
	}
	r.attachConn(conn, true)
	return nil
}

func (r *Client) startServer(ctx clientContext) error {
	lockFile, err := os.OpenFile(ctx.lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if connectErr := r.connectExisting(ctx.serverAddr, 5*time.Second); connectErr == nil {
			return nil
		}
		_ = os.Remove(ctx.lockPath)
		lockFile, err = os.OpenFile(ctx.lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("acquire runtime server lock: %w", err)
		}
	}
	_ = lockFile.Close()

	logFile, err := os.OpenFile(ctx.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		_ = os.Remove(ctx.lockPath)
		return fmt.Errorf("open runtime log: %w", err)
	}

	cmd := exec.Command(ctx.node, ctx.runtimePath)
	cmd.Dir = ctx.appRoot
	cmd.Env = append(os.Environ(),
		"QUBIT_WORKSPACE_CWD="+ctx.launchCwd,
		"QUBIT_PROJECT_DIR="+ctx.qubitDir,
		"QUBIT_RUNTIME_ADDR="+ctx.serverAddr,
		"QUBIT_RUNTIME_LOCK_PATH="+ctx.lockPath,
	)
	cmd.Stderr = logFile
	configureDetachedProcess(cmd)

	r.mu.Lock()
	r.cmd = cmd
	r.attached = false
	r.mu.Unlock()

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		r.cleanupFailedStart()
		return err
	}
	_ = logFile.Close()
	if cmd.Process != nil {
		_ = os.WriteFile(ctx.lockPath, []byte(fmt.Sprintf("%d\n%s\n", cmd.Process.Pid, ctx.serverAddr)), 0644)
	}

	conn, err := connectRuntimeServer(ctx.serverAddr, 5*time.Second)
	if err != nil {
		r.cleanupFailedStart()
		return fmt.Errorf("connect runtime server: %w", err)
	}
	r.attachConn(conn, false)
	go r.waitRuntimeProcess(cmd)
	return nil
}

func (r *Client) waitRuntimeProcess(cmd *exec.Cmd) {
	_ = cmd.Wait()
}

func runtimeServerAddress(qubitDir string) string {
	sum := sha1.Sum([]byte(strings.ToLower(filepath.Clean(qubitDir))))
	portOffset := int(sum[0])<<8 | int(sum[1])
	port := 20000 + portOffset%30000
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func connectRuntimeServer(address string, timeout time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		conn, err := net.DialTimeout("tcp", address, 150*time.Millisecond)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		time.Sleep(75 * time.Millisecond)
	}
}

func (r *Client) attachConn(conn io.ReadWriteCloser, attached bool) {
	r.mu.Lock()
	r.conn = conn
	r.stdin = conn
	r.attached = attached
	r.closed = false
	r.connSeq++
	seq := r.connSeq
	r.mu.Unlock()
	go r.readStdout(conn, seq)
}

func (r *Client) readStdout(stdout io.Reader, seq int) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if !r.shouldReportReader(seq) {
			return
		}
		line := append([]byte(nil), scanner.Bytes()...)
		var ev protocol.RuntimeEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			r.appendLog("stdout", string(line))
			r.emitError(fmt.Errorf("bad runtime event: %s", string(line)))
			continue
		}
		r.appendLog("stdout", summarizeRuntimeEvent(line, ev))
		r.events <- ev
	}
	if !r.shouldReportReader(seq) {
		return
	}
	if err := scanner.Err(); err != nil {
		r.emitError(fmt.Errorf("%w: %v", ErrDisconnected, err))
		return
	}
	r.emitError(ErrDisconnected)
}

func (r *Client) shouldReportReader(seq int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.closed && r.connSeq == seq
}

func (r *Client) emitError(err error) {
	select {
	case r.errs <- err:
	default:
	}
}

func (r *Client) appendLog(stream string, line string) {
	if r.logPath == "" {
		return
	}
	entry := fmt.Sprintf("[%s] %s\n", stream, line)
	f, err := os.OpenFile(r.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(entry)
}

func summarizeRuntimeEvent(raw []byte, ev protocol.RuntimeEvent) string {
	switch ev.Type {
	case "session.list":
		return fmt.Sprintf("%s id=%s sessionId=%s sessionTitle=%q sessions=%d", ev.Type, ev.ID, ev.SessionID, ev.SessionTitle, len(ev.Sessions))
	case "session.messages":
		return fmt.Sprintf("%s id=%s sessionId=%s sessionTitle=%q messages=%d", ev.Type, ev.ID, ev.SessionID, ev.SessionTitle, len(ev.Messages))
	case "session.tree":
		return fmt.Sprintf("%s id=%s sessionId=%s sessionTitle=%q nodes=%d", ev.Type, ev.ID, ev.SessionID, ev.SessionTitle, len(ev.ForkTreeNodes))
	}
	return string(raw)
}

func (r *Client) closeConnectionOnly(markClosed bool) {
	r.mu.Lock()
	if markClosed {
		r.closed = true
	}
	conn := r.conn
	r.conn = nil
	r.stdin = nil
	r.connSeq++
	r.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (r *Client) cleanupFailedStart() {
	r.closeConnectionOnly(false)
	r.mu.Lock()
	cmd := r.cmd
	lockPath := r.lockPath
	r.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	if lockPath != "" {
		_ = os.Remove(lockPath)
	}
}

func (r *Client) shutdown() {
	r.closeConnectionOnly(true)
}

func (r *Client) send(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	r.mu.Lock()
	stdin := r.stdin
	closed := r.closed
	r.mu.Unlock()
	if closed || stdin == nil {
		return ErrDisconnected
	}
	_, err = fmt.Fprintf(stdin, "%s\n", payload)
	return err
}

func (r *Client) Reconnect() error {
	r.closeConnectionOnly(false)
	ctx, err := r.context()
	if err != nil {
		return err
	}
	if err := r.connectExisting(ctx.serverAddr, 750*time.Millisecond); err == nil {
		return nil
	}
	return r.startServer(ctx)
}

func (r *Client) Events() <-chan protocol.RuntimeEvent { return r.events }
func (r *Client) Errors() <-chan error                 { return r.errs }
func (r *Client) LaunchCwd() string                    { return r.launchCwd }
func (r *Client) SetLaunchCwd(cwd string)              { r.launchCwd = cwd }
func (r *Client) QubitDir() string                     { return r.qubitDir }
func (r *Client) LogPath() string                      { return r.logPath }

func (r *Client) Shutdown()        { r.shutdown() }
func (r *Client) Send(v any) error { return r.send(v) }

func findAppRoot() (string, error) {
	candidates := []string{}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, exeDir, filepath.Dir(exeDir))
	}

	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		if isAppRoot(abs) {
			return abs, nil
		}
	}

	return "", fmt.Errorf("could not find Qubit app root. Run from the Qubit project root or keep the built binary under the project root")
}

func isAppRoot(dir string) bool {
	if hasFile(dir, "package.json") && hasFile(dir, "go.mod") {
		return true
	}
	if hasFile(dir, filepath.Join("dist", "runtime.js")) {
		return true
	}
	if hasFile(dir, "runtime.ts") {
		return true
	}
	return hasFile(dir, "runtime.mjs")
}

func hasFile(dir, name string) bool {
	info, err := os.Stat(filepath.Join(dir, name))
	return err == nil && !info.IsDir()
}
