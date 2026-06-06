package tui

import (
	"io"
	"os/exec"

	"github.com/qubit/graviton-cli/internal/tui/runtimeclient"
)

type runtimeClient struct {
	client *runtimeclient.Client

	cmd       *exec.Cmd
	conn      io.ReadWriteCloser
	stdin     io.WriteCloser
	events    <-chan runtimeEvent
	errs      <-chan error
	appRoot   string
	launchCwd string
	qubitDir  string
	logPath   string
	lockPath  string
	attached  bool
	clientID  string
	reconnect func() error
}
