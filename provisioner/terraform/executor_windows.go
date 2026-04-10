//go:build windows

package terraform

import (
	"os"
	"syscall"
)

// cmdSysProcAttr returns nil on Windows because process group
// semantics differ from POSIX. The default behavior is sufficient.
func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

// signalProcessGroup falls back to signaling only the direct
// process on Windows, which lacks POSIX-style process group
// signaling.
func signalProcessGroup(pid int, sig os.Signal) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(sig)
}
