package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/qubit/graviton-cli/internal/tui/runtimeclient"
)

func startRuntime() (*runtimeClient, error) {
	client, err := runtimeclient.Start()
	if err != nil {
		return nil, err
	}
	return &runtimeClient{
		client:    client,
		events:    client.Events(),
		errs:      client.Errors(),
		launchCwd: client.LaunchCwd(),
		qubitDir:  client.QubitDir(),
		logPath:   client.LogPath(),
	}, nil
}

func (r *runtimeClient) shutdown() {
	if r == nil {
		return
	}
	if r.client != nil {
		r.client.Shutdown()
		return
	}
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
	if r.client != nil {
		return r.client.Send(v)
	}
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
