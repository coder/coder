//go:build !windows
// +build !windows

package pty

import (
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

func startPty(cmd *exec.Cmd) (PTY, error) {
	pty, tty, err := pty.Open()
	if err != nil {
		return nil, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}
	cmd.Stdout = tty
	cmd.Stderr = tty
	cmd.Stdin = tty
	err = cmd.Start()
	if err != nil {
		_ = pty.Close()
		return nil, err
	}
	return &otherPty{
		pty: pty,
		tty: tty,
	}, nil
}
