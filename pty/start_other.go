//go:build !windows
// +build !windows

package pty

import (
	"os/exec"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/xerrors"
)

func startPty(cmd *exec.Cmd) (PTY, error) {
	ptty, tty, err := pty.Open()
	if err != nil {
		return nil, xerrors.Errorf("open: %w", err)
	}
	defer func() {
		_ = tty.Close()
	}()
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
		return nil, xerrors.Errorf("start: %w", err)
	}
	return &otherPty{
		pty: ptty,
		tty: tty,
	}, nil
}
