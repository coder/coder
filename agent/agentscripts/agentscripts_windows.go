package agentscripts

import (
	"os/exec"
	"syscall"
)

func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func cmdCancel(cmd *exec.Cmd) func() error {
	return func() error {
		return nil
	}
}
