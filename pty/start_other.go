//go:build !windows
// +build !windows

package pty

import (
	"os/exec"

	"github.com/creack/pty"
)

func runPty(cmd *exec.Cmd) (Pty, error) {
	pty, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	return &otherPty{pty, pty}, nil
}
