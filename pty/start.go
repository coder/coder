package pty

import "os/exec"

func Start(cmd *exec.Cmd) (PTY, error) {
	return startPty(cmd)
}
