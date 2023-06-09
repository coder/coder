//go:build windows

package pty

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"golang.org/x/xerrors"
)

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procResizePseudoConsole = kernel32.NewProc("ResizePseudoConsole")
	procCreatePseudoConsole = kernel32.NewProc("CreatePseudoConsole")
	procClosePseudoConsole  = kernel32.NewProc("ClosePseudoConsole")
)

// See: https://docs.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session
func newPty(opt ...Option) (*ptyWindows, error) {
	var opts ptyOptions
	for _, o := range opt {
		o(&opts)
	}

	// We use the CreatePseudoConsole API which was introduced in build 17763
	vsn := windows.RtlGetVersion()
	if vsn.MajorVersion < 10 ||
		vsn.BuildNumber < 17763 {
		// If the CreatePseudoConsole API is not available, we fall back to a simpler
		// implementation that doesn't create an actual PTY - just uses os.Pipe
		return nil, xerrors.Errorf("pty not supported")
	}

	pty := &ptyWindows{
		opts: opts,
	}

	var err error
	pty.inputRead, pty.inputWrite, err = os.Pipe()
	if err != nil {
		return nil, err
	}
	pty.outputRead, pty.outputWrite, err = os.Pipe()
	if err != nil {
		_ = pty.inputRead.Close()
		_ = pty.inputWrite.Close()
		return nil, err
	}

	consoleSize := uintptr(80) + (uintptr(80) << 16)
	if opts.sshReq != nil {
		consoleSize = uintptr(opts.sshReq.Window.Width) + (uintptr(opts.sshReq.Window.Height) << 16)
	}
	ret, _, err := procCreatePseudoConsole.Call(
		consoleSize,
		uintptr(pty.inputRead.Fd()),
		uintptr(pty.outputWrite.Fd()),
		0,
		uintptr(unsafe.Pointer(&pty.console)),
	)
	if int32(ret) < 0 {
		_ = pty.Close()
		return nil, xerrors.Errorf("create pseudo console (%d): %w", int32(ret), err)
	}
	return pty, nil
}

type ptyWindows struct {
	opts    ptyOptions
	console windows.Handle

	outputWrite *os.File
	outputRead  *os.File
	inputWrite  *os.File
	inputRead   *os.File

	closeMutex sync.Mutex
	closed     bool
}

type windowsProcess struct {
	// cmdDone protects access to cmdErr: anything reading cmdErr should read from cmdDone first.
	cmdDone chan any
	cmdErr  error
	proc    *os.Process
	pw      *ptyWindows
}

// Name returns the TTY name on Windows.
//
// Not implemented.
func (p *ptyWindows) Name() string {
	return ""
}

func (p *ptyWindows) Output() ReadWriter {
	return ReadWriter{
		Reader: p.outputRead,
		Writer: p.outputWrite,
	}
}

func (p *ptyWindows) OutputReader() io.Reader {
	return p.outputRead
}

func (p *ptyWindows) Input() ReadWriter {
	return ReadWriter{
		Reader: p.inputRead,
		Writer: p.inputWrite,
	}
}

func (p *ptyWindows) InputWriter() io.Writer {
	return p.inputWrite
}

func (p *ptyWindows) Resize(height uint16, width uint16) error {
	// hold the lock, so we don't race with anyone trying to close the console
	p.closeMutex.Lock()
	defer p.closeMutex.Unlock()
	if p.closed || p.console == windows.InvalidHandle {
		return ErrClosed
	}
	// Taken from: https://github.com/microsoft/hcsshim/blob/54a5ad86808d761e3e396aff3e2022840f39f9a8/internal/winapi/zsyscall_windows.go#L144
	ret, _, err := procResizePseudoConsole.Call(uintptr(p.console), uintptr(*((*uint32)(unsafe.Pointer(&windows.Coord{
		Y: int16(height),
		X: int16(width),
	})))))
	if ret != 0 {
		return err
	}
	return nil
}

func (p *ptyWindows) Close() error {
	p.closeMutex.Lock()
	defer p.closeMutex.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true

	// if we are running a command in the PTY, the corresponding *windowsProcess
	// may have already closed the PseudoConsole when the command exited, so that
	// output reads can get to EOF.  In that case, we don't need to close it
	// again here.
	if p.console != windows.InvalidHandle {
		ret, _, err := procClosePseudoConsole.Call(uintptr(p.console))
		if ret < 0 {
			return xerrors.Errorf("close pseudo console: %w", err)
		}
		p.console = windows.InvalidHandle
	}

	// We always have these files
	_ = p.outputRead.Close()
	_ = p.inputWrite.Close()
	// These get closed & unset if we Start() a new process.
	if p.outputWrite != nil {
		_ = p.outputWrite.Close()
	}
	if p.inputRead != nil {
		_ = p.inputRead.Close()
	}
	return nil
}

func (p *windowsProcess) waitInternal() {
	// put this on the bottom of the defer stack since the next defer can write to p.cmdErr
	defer close(p.cmdDone)
	defer func() {
		// close the pseudoconsole handle when the process exits, if it hasn't already been closed.
		// this is important because the PseudoConsole (conhost.exe) holds the write-end
		// of the output pipe.  If it is not closed, reads on that pipe will block, even though
		// the command has exited.
		// c.f. https://devblogs.microsoft.com/commandline/windows-command-line-introducing-the-windows-pseudo-console-conpty/
		p.pw.closeMutex.Lock()
		defer p.pw.closeMutex.Unlock()
		if p.pw.console != windows.InvalidHandle {
			ret, _, err := procClosePseudoConsole.Call(uintptr(p.pw.console))
			if ret < 0 && p.cmdErr == nil {
				// if we already have an error from the command, prefer that error
				// but if the command succeeded and closing the PseudoConsole fails
				// then record that error so that we have a chance to see it
				p.cmdErr = err
			}
			p.pw.console = windows.InvalidHandle
		}
	}()

	state, err := p.proc.Wait()
	if err != nil {
		p.cmdErr = err
		return
	}
	if !state.Success() {
		p.cmdErr = &exec.ExitError{ProcessState: state}
		return
	}
}

func (p *windowsProcess) Wait() error {
	<-p.cmdDone
	return p.cmdErr
}

func (p *windowsProcess) Kill() error {
	return p.proc.Kill()
}

// killOnContext waits for the context to be done and kills the process, unless it exits on its own first.
func (p *windowsProcess) killOnContext(ctx context.Context) {
	select {
	case <-p.cmdDone:
		return
	case <-ctx.Done():
		p.Kill()
	}
}
