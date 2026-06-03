package main

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

	tea "charm.land/bubbletea/v2"
)

func startRuntime() (*runtimeClient, error) {
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
		rt := &runtimeClient{conn: conn, stdin: conn, events: make(chan runtimeEvent, 32), errs: make(chan error, 4), appRoot: appRoot, launchCwd: launchCwd, qubitDir: qubitDir, logPath: logPath, lockPath: lockPath, attached: true}
		go rt.readStdout(conn)
		return rt, nil
	}

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if conn, connectErr := connectRuntimeServer(serverAddr, 5*time.Second); connectErr == nil {
			rt := &runtimeClient{conn: conn, stdin: conn, events: make(chan runtimeEvent, 32), errs: make(chan error, 4), appRoot: appRoot, launchCwd: launchCwd, qubitDir: qubitDir, logPath: logPath, lockPath: lockPath, attached: true}
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
	rt := &runtimeClient{cmd: cmd, events: make(chan runtimeEvent, 32), errs: make(chan error, 4), appRoot: appRoot, launchCwd: launchCwd, qubitDir: qubitDir, logPath: logPath, lockPath: lockPath}
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

func (r *runtimeClient) readStdout(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		r.appendLog("stdout", string(line))
		var ev runtimeEvent
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

func (r *runtimeClient) readStderr(stderr io.Reader) {
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

func (r *runtimeClient) appendLog(stream string, line string) {
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

func (r *runtimeClient) shutdown() {
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

func (r *runtimeClient) send(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(r.stdin, "%s\n", payload)
	return err
}

func waitRuntimeEvent(r *runtimeClient) tea.Cmd {
	return func() tea.Msg {
		select {
		case ev, ok := <-r.events:
			if !ok {
				return runtimeErrMsg{err: fmt.Errorf("runtime stopped")}
			}
			return runtimeMsg(ev)
		case err := <-r.errs:
			return runtimeErrMsg{err: err}
		}
	}
}

func sendRuntime(r *runtimeClient, payload map[string]any) tea.Cmd {
	return func() tea.Msg {
		if _, ok := payload["id"]; !ok {
			payload["id"] = fmt.Sprintf("msg_%d", time.Now().UnixNano())
		}
		err := r.send(payload)
		return sendDoneMsg{err: err}
	}
}
