package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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
	logPath := filepath.Join(appRoot, ".qubit", "runtime.log")
	_ = os.MkdirAll(filepath.Dir(logPath), 0755)
	_ = os.WriteFile(logPath, []byte(""), 0644)

	runtimePath := filepath.Join(appRoot, "dist", "runtime.js")
	if _, err := os.Stat(runtimePath); err != nil {
		return nil, fmt.Errorf("runtime not built at %s; run pnpm run build:runtime", runtimePath)
	}
	cmd := exec.Command(node, runtimePath)
	cmd.Dir = appRoot
	cmd.Env = append(os.Environ(), "QUBIT_WORKSPACE_CWD="+launchCwd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	rt := &runtimeClient{cmd: cmd, stdin: stdin, events: make(chan runtimeEvent, 32), errs: make(chan error, 4), appRoot: appRoot, launchCwd: launchCwd, logPath: logPath}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go rt.readStdout(stdout)
	go rt.readStderr(stderr)
	go func() {
		if err := cmd.Wait(); err != nil {
			rt.errs <- fmt.Errorf("runtime exited: %w", err)
		}
		close(rt.events)
	}()

	return rt, nil
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
			if strings.HasPrefix(line, "[codex-oauth]") {
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
	_ = r.send(map[string]any{"type": "shutdown"})
	_ = r.stdin.Close()
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
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
