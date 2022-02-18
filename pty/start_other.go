//go:build !windows
// +build !windows

package pty

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

func startPty(cmd *exec.Cmd) (PTY, *os.Process, error) {
	ptty, tty, err := pty.Open()
	if err != nil {
		return nil, nil, err
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
		return nil, nil, err
	}
	oPty := &otherPty{
		pty: ptty,
		tty: tty,
	}
	go func() {
		_ = cmd.Wait()
		_ = oPty.Close()
	}()
	return oPty, cmd.Process, nil
}
