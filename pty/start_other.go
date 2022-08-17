//go:build !windows
// +build !windows

package pty

import (
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/xerrors"
)

func startPty(cmd *exec.Cmd) (PTY, Process, error) {
	ptty, tty, err := pty.Open()
	if err != nil {
		return nil, nil, xerrors.Errorf("open: %w", err)
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
		_ = tty.Close()
		if runtime.GOOS == "darwin" && strings.Contains(err.Error(), "bad file descriptor") {
			// macOS has an obscure issue where the PTY occasionally closes
			// before it's used. It's unknown why this is, but creating a new
			// TTY resolves it.
			return startPty(cmd)
		}
		return nil, nil, xerrors.Errorf("start: %w", err)
	}
	oPty := &otherPty{
		pty: ptty,
		tty: tty,
	}
	oProcess := &otherProcess{
		pty:     ptty,
		cmd:     cmd,
		cmdDone: make(chan any),
	}
	go oProcess.waitInternal()
	return oPty, oProcess, nil
}
