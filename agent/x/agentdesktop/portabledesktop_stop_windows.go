package agentdesktop

import "os"

// interruptRecordingProcess kills the recording process directly
// because os.Process.Signal(os.Interrupt) is not supported on
// Windows and returns an error without delivering a signal.
func interruptRecordingProcess(p *os.Process) error {
	return p.Kill()
}
