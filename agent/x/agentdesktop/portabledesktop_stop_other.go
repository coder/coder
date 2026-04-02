//go:build !windows

package agentdesktop

import "os"

// interruptRecordingProcess sends a SIGINT to the recording process
// for graceful shutdown. On Unix, os.Interrupt is delivered as
// SIGINT which lets the recorder finalize the MP4 container.
func interruptRecordingProcess(p *os.Process) error {
	return p.Signal(os.Interrupt)
}
