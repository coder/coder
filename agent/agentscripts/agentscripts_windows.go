package agentscripts

import "syscall"

func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
