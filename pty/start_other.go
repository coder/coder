//go:build !windows

package pty

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"syscall"

	"golang.org/x/xerrors"
)

func startPty(cmdPty *Cmd, opt ...StartOption) (retPTY *otherPty, proc Process, err error) {
	var opts startOptions
	for _, o := range opt {
		o(&opts)
	}

	opty, err := newPty(opts.ptyOpts...)
	if err != nil {
		return nil, nil, xerrors.Errorf("newPty failed: %w", err)
	}

	origEnv := cmdPty.Env
	if opty.opts.sshReq != nil {
		cmdPty.Env = append(cmdPty.Env, fmt.Sprintf("SSH_TTY=%s", opty.Name()))
	}
	if opty.opts.setGPGTTY {
		cmdPty.Env = append(cmdPty.Env, fmt.Sprintf("GPG_TTY=%s", opty.Name()))
	}
	if cmdPty.Context == nil {
		cmdPty.Context = context.Background()
	}
	cmdExec := cmdPty.AsExec()

	cmdExec.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}
	cmdExec.Stdout = opty.tty
	cmdExec.Stderr = opty.tty
	cmdExec.Stdin = opty.tty
	err = cmdExec.Start()
	if err != nil {
		_ = opty.Close()
		if runtime.GOOS == "darwin" && strings.Contains(err.Error(), "bad file descriptor") {
			// macOS has an obscure issue where the PTY occasionally closes
			// before it's used. It's unknown why this is, but creating a new
			// TTY resolves it.
			cmdPty.Env = origEnv
			return startPty(cmdPty, opt...)
		}
		return nil, nil, xerrors.Errorf("start: cmd %q: %w", cmdPty.Args, err)
	}
	if runtime.GOOS == "linux" {
		// Now that we've started the command, and passed the TTY to it, close
		// our file so that the other process has the only open file to the TTY.
		// Once the process closes the TTY (usually on exit), there will be no
		// open references and the OS kernel returns an error when trying to
		// read or write to our PTY end.  Without this (on Linux), reading from
		// the process output will block until we close our TTY.
		//
		// Note that on darwin, reads on the PTY don't block even if we keep the
		// TTY file open, and keeping it open seems to prevent race conditions
		// where we lose output.  Couldn't find official documentation
		// confirming this, but I did find a thread of someone else's
		// observations: https://developer.apple.com/forums/thread/663632
		if err := opty.tty.Close(); err != nil {
			_ = cmdExec.Process.Kill()
			return nil, nil, xerrors.Errorf("close tty: %w", err)
		}
		opty.tty = nil // remove so we don't attempt to close it again.
	}
	oProcess := &otherProcess{
		pty:     opty.pty,
		cmd:     cmdExec,
		cmdDone: make(chan any),
	}
	go oProcess.waitInternal()
	return opty, oProcess, nil
}
