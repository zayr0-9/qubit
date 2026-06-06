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
		clientID:  client.ClientID(),
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
	if r.conn != nil {
		_ = r.conn.Close()
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

func reconnectRuntime(r *runtimeClient) tea.Cmd {
	return func() tea.Msg {
		if r == nil {
			return runtimeReconnectMsg{err: fmt.Errorf("runtime client unavailable")}
		}
		if r.client != nil {
			err := r.client.Reconnect()
			return runtimeReconnectMsg{err: err}
		}
		if r.reconnect != nil {
			return runtimeReconnectMsg{err: r.reconnect()}
		}
		return runtimeReconnectMsg{err: fmt.Errorf("runtime reconnect unavailable")}
	}
}

func runtimeQubitDir(r *runtimeClient) string {
	if r == nil {
		return ""
	}
	if _, ok := os.LookupEnv("QUBIT_CONFIG_DIR"); !ok && r.client == nil && r.qubitDir != "" {
		return ""
	}
	return r.qubitDir
}
