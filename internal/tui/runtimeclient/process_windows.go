package runtimeclient

import (
	"os/exec"
	"syscall"
)

const detachedProcess = 0x00000008
const createNewProcessGroup = 0x00000200

func configureDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNewProcessGroup,
	}
}
