//go:build !windows

package agentscripts

import "syscall"

func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}
