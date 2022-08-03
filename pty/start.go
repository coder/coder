package pty

import (
	"os/exec"
)

// Start the command in a TTY.  The calling code must not use cmd after passing it to the PTY, and
// instead rely on the returned Process to manage the command/process.
func Start(cmd *exec.Cmd) (PTY, Process, error) {
	return startPty(cmd)
}
