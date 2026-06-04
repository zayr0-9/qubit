package runtimeclient

import (
	"bufio"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qubit/graviton-cli/internal/tui/protocol"
)

type Client struct {
	cmd       *exec.Cmd
	conn      io.ReadWriteCloser
	stdin     io.WriteCloser
	events    chan protocol.RuntimeEvent
	errs      chan error
	appRoot   string
	launchCwd string
	qubitDir  string
	logPath   string
	lockPath  string
	attached  bool
}

func Start() (*Client, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, err
	}
	launchCwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get launch cwd: %w", err)
	}
	appRoot, err := findAppRoot()
	if err != nil {
		return nil, err
	}
	qubitDir := filepath.Join(launchCwd, ".qubit")
	if err := os.MkdirAll(qubitDir, 0755); err != nil {
		return nil, fmt.Errorf("create project .qubit directory: %w", err)
	}
	logPath := filepath.Join(qubitDir, "runtime.log")
	runtimePath := filepath.Join(appRoot, "dist", "runtime.js")
	if _, err := os.Stat(runtimePath); err != nil {
		return nil, fmt.Errorf("runtime not built at %s; run pnpm run build:runtime", runtimePath)
	}

	serverAddr := runtimeServerAddress(qubitDir)
	lockPath := filepath.Join(qubitDir, "runtime-server.lock")
	if conn, err := connectRuntimeServer(serverAddr, 150*time.Millisecond); err == nil {
		rt := &Client{conn: conn, stdin: conn, events: make(chan protocol.RuntimeEvent, 32), errs: make(chan error, 4), appRoot: appRoot, launchCwd: launchCwd, qubitDir: qubitDir, logPath: logPath, lockPath: lockPath, attached: true}
		go rt.readStdout(conn)
		return rt, nil
	}

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if conn, connectErr := connectRuntimeServer(serverAddr, 5*time.Second); connectErr == nil {
			rt := &Client{conn: conn, stdin: conn, events: make(chan protocol.RuntimeEvent, 32), errs: make(chan error, 4), appRoot: appRoot, launchCwd: launchCwd, qubitDir: qubitDir, logPath: logPath, lockPath: lockPath, attached: true}
			go rt.readStdout(conn)
			return rt, nil
		}
		_ = os.Remove(lockPath)
		lockFile, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("acquire runtime server lock: %w", err)
		}
	}
	_, _ = fmt.Fprintf(lockFile, "%d\n%s\n", os.Getpid(), serverAddr)
	_ = lockFile.Close()

	_ = os.WriteFile(logPath, []byte(""), 0644)
	cmd := exec.Command(node, runtimePath)
	cmd.Dir = appRoot
	cmd.Env = append(os.Environ(), "QUBIT_WORKSPACE_CWD="+launchCwd, "QUBIT_PROJECT_DIR="+qubitDir, "QUBIT_RUNTIME_ADDR="+serverAddr)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	rt := &Client{cmd: cmd, events: make(chan protocol.RuntimeEvent, 32), errs: make(chan error, 4), appRoot: appRoot, launchCwd: launchCwd, qubitDir: qubitDir, logPath: logPath, lockPath: lockPath}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go rt.readStderr(stderr)

	conn, err := connectRuntimeServer(serverAddr, 5*time.Second)
	if err != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("connect runtime server: %w", err)
	}
	rt.conn = conn
	rt.stdin = conn
	go rt.readStdout(conn)
	go func() {
		if err := cmd.Wait(); err != nil {
			rt.errs <- fmt.Errorf("runtime exited: %w", err)
		}
		close(rt.events)
	}()
	return rt, nil
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

func (r *Client) readStdout(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		r.appendLog("stdout", string(line))
		var ev protocol.RuntimeEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			r.errs <- fmt.Errorf("bad runtime event: %s", string(line))
			continue
		}
		r.events <- ev
	}
	if err := scanner.Err(); err != nil {
		r.errs <- err
	}
}

func (r *Client) readStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			r.appendLog("stderr", line)
			// Opt-in OAuth diagnostics are intentionally written to stderr by the
			// Node runtime, but they are not runtime failures. Keep them in
			// .qubit/runtime.log without interrupting the TUI or hiding the auth URL.
			if strings.HasPrefix(line, "[codex-oauth]") || strings.HasPrefix(line, "[codex-retry]") || strings.HasPrefix(line, "[codex-reasoning]") || strings.HasPrefix(line, "[runtime-server]") {
				continue
			}
			r.errs <- fmt.Errorf("%s", line)
		}
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

func (r *Client) shutdown() {
	if r.attached {
		if r.conn != nil {
			_ = r.conn.Close()
		}
		return
	}
	if r.conn != nil {
		_ = r.conn.Close()
	}
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
	if r.lockPath != "" {
		_ = os.Remove(r.lockPath)
	}
}

func (r *Client) send(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(r.stdin, "%s\n", payload)
	return err
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

	return "", fmt.Errorf("could not find Qubit app root. Run from D:\\qubit or keep bin\\qubit.exe under the project root")
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
