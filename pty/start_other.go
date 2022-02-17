//go:build !windows
// +build !windows

package pty

import (
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

func startPty(cmd *exec.Cmd) (PTY, error) {
	ptty, tty, err := pty.Open()
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
		_ = ptty.Close()
		return nil, err
	}
	return &otherPty{
		pty: ptty,
		tty: tty,
	}, nil
}
