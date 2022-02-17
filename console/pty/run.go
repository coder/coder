package pty

import "os/exec"

func Run(cmd *exec.Cmd) (Pty, error) {
	return runPty(cmd)
}
