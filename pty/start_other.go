//go:build !windows

package pty

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"golang.org/x/xerrors"
)

func startPty(cmd *exec.Cmd, opt ...StartOption) (retPTY *otherPty, proc Process, err error) {
	var opts startOptions
	for _, o := range opt {
		o(&opts)
	}

	opty, err := newPty(opts.ptyOpts...)
	if err != nil {
		return nil, nil, xerrors.Errorf("newPty failed: %w", err)
	}

	origEnv := cmd.Env
	if opty.opts.sshReq != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("SSH_TTY=%s", opty.Name()))
	}
	if opty.opts.setGPGTTY {
		cmd.Env = append(cmd.Env, fmt.Sprintf("GPG_TTY=%s", opty.Name()))
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}
	cmd.Stdout = opty.tty
	cmd.Stderr = opty.tty
	cmd.Stdin = opty.tty
	err = cmd.Start()
	if err != nil {
		_ = opty.Close()
		if runtime.GOOS == "darwin" && strings.Contains(err.Error(), "bad file descriptor") {
			// macOS has an obscure issue where the PTY occasionally closes
			// before it's used. It's unknown why this is, but creating a new
			// TTY resolves it.
			cmd.Env = origEnv
			return startPty(cmd, opt...)
		}
		return nil, nil, xerrors.Errorf("start: %w", err)
	}
	oProcess := &otherProcess{
		pty:     opty.pty,
		cmd:     cmd,
		cmdDone: make(chan any),
	}
	go oProcess.waitInternal()
	return opty, oProcess, nil
}
