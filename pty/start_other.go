//go:build !windows
// +build !windows

package pty

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/xerrors"
)

func startPty(cmd *exec.Cmd) (PTY, *os.Process, error) {
	ptty, tty, err := pty.Open()
	if err != nil {
		return nil, nil, xerrors.Errorf("open: %w", err)
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
		return nil, nil, xerrors.Errorf("start: %w", err)
	}
	oPty := &otherPty{
		pty: ptty,
		tty: tty,
	}
	return oPty, cmd.Process, nil
}
