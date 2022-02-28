package pty

import (
	"os"
	"os/exec"
)

func Start(cmd *exec.Cmd) (PTY, *os.Process, error) {
	return startPty(cmd)
}
