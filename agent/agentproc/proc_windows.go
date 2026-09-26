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

// terminatedByKill reports true because Windows exit status does not
// record what ended the process, so a successful kill is taken as the
// cause.
func terminatedByKill(*os.ProcessState) bool {
	return true
}
