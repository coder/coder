//go:build !windows

package pty

import (
	"os"
	"os/exec"
	"runtime"
	"sync"

	"github.com/creack/pty"
	"github.com/u-root/u-root/pkg/termios"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

func newPty(opt ...Option) (retPTY *otherPty, err error) {
	var opts ptyOptions
	for _, o := range opt {
		o(&opts)
	}

	ptyFile, ttyFile, err := pty.Open()
	if err != nil {
		return nil, err
	}
	opty := &otherPty{
		pty:  ptyFile,
		tty:  ttyFile,
		opts: opts,
	}
	defer func() {
		if err != nil {
			_ = opty.Close()
		}
	}()

	if opts.sshReq != nil {
		err = opty.control(opty.tty, func(fd uintptr) error {
			return applyTerminalModesToFd(opts.logger, fd, *opts.sshReq)
		})
		if err != nil {
			return nil, err
		}
	}

	return opty, nil
}

type otherPty struct {
	mutex    sync.Mutex
	closed   bool
	err      error
	pty, tty *os.File
	opts     ptyOptions
}

func (p *otherPty) control(tty *os.File, fn func(fd uintptr) error) (err error) {
	defer func() {
		// Always echo the close error for closed ptys.
		p.mutex.Lock()
		defer p.mutex.Unlock()
		if p.closed {
			err = p.err
		}
	}()

	rawConn, err := tty.SyscallConn()
	if err != nil {
		return err
	}

	var ctlErr error
	err = rawConn.Control(func(fd uintptr) {
		ctlErr = fn(fd)
	})
	switch {
	case err != nil:
		return err
	case ctlErr != nil:
		return ctlErr
	default:
		return nil
	}
}

func (p *otherPty) Name() string {
	return p.tty.Name()
}

func (p *otherPty) Input() ReadWriter {
	return ReadWriter{
		Reader: p.tty,
		Writer: p.pty,
	}
}

func (p *otherPty) Output() ReadWriter {
	return ReadWriter{
		Reader: p.pty,
		Writer: p.tty,
	}
}

func (p *otherPty) Resize(height uint16, width uint16) error {
	return p.control(p.pty, func(fd uintptr) error {
		return termios.SetWinSize(fd, &termios.Winsize{
			Winsize: unix.Winsize{
				Row: height,
				Col: width,
			},
		})
	})
}

func (p *otherPty) Close() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return p.err
	}
	p.closed = true

	err := p.pty.Close()
	err2 := p.tty.Close()
	if err == nil {
		err = err2
	}

	if err != nil {
		p.err = err
	} else {
		p.err = xerrors.New("pty: closed")
	}

	return err
}

type otherProcess struct {
	pty *os.File
	cmd *exec.Cmd

	// cmdDone protects access to cmdErr: anything reading cmdErr should read from cmdDone first.
	cmdDone chan any
	cmdErr  error
}

func (p *otherProcess) Wait() error {
	<-p.cmdDone
	return p.cmdErr
}

func (p *otherProcess) Kill() error {
	return p.cmd.Process.Kill()
}

func (p *otherProcess) waitInternal() {
	// The GC can garbage collect the TTY FD before the command
	// has finished running. See:
	// https://github.com/creack/pty/issues/127#issuecomment-932764012
	p.cmdErr = p.cmd.Wait()
	runtime.KeepAlive(p.pty)
	close(p.cmdDone)
}
