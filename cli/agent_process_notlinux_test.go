//go:build !linux

package cli_test

import (
	"os"
	"os/exec"
)

func configureProcessGroup(*exec.Cmd) {}

func interruptProcessGroup(command *exec.Cmd) error {
	return command.Process.Signal(os.Interrupt)
}

func killProcessGroup(command *exec.Cmd) error {
	return command.Process.Kill()
}
