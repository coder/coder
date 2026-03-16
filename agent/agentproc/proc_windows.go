package agentproc

import (
	"os"
	"syscall"
)

// procSysProcAttr returns the SysProcAttr to use when spawning
// processes. On Windows, process groups are not supported in the
// same way as Unix, so this returns an empty struct.
func procSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

// signalProcess sends a signal directly to the process. Windows
// does not support process group signaling, so we fall back to
// sending the signal to the process itself.
func signalProcess(p *os.Process, _ syscall.Signal) error {
	return p.Kill()
}
