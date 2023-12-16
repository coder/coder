//go:build !windows

package agentscripts

import (
	"os/exec"
	"syscall"
)

func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}

func cmdCancel(cmd *exec.Cmd) func() error {
	return func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP)
	}
}
