//go:build !windows
// +build !windows

package pty

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/xerrors"
)

func startPty(cmd *exec.Cmd) (PTY, *os.Process, error) {
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
		if runtime.GOOS == "darwin" && strings.Contains(err.Error(), "bad file descriptor") {
			// MacOS has an obscure issue where the PTY occasionally closes
			// before it's used. It's unknown why this is, but creating a new
			// TTY resolves it.
			return startPty(cmd)
		}
		return nil, nil, xerrors.Errorf("start: %w", err)
	}
	go func() {
		// The GC can garbage collect the TTY FD before the command
		// has finished running. See:
		// https://github.com/creack/pty/issues/127#issuecomment-932764012
		_ = cmd.Wait()
		runtime.KeepAlive(ptty)
	}()
	oPty := &otherPty{
		pty: ptty,
		tty: tty,
	}
	return oPty, cmd.Process, nil
}
