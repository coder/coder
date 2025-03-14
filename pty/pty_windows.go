//go:build windows
package pty
import (
	"fmt"
	"errors"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"unsafe"
	"golang.org/x/sys/windows"
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
		return nil, fmt.Errorf("pty not supported")
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
	// CreatePseudoConsole returns S_OK on success, as per:
	// https://learn.microsoft.com/en-us/windows/console/createpseudoconsole
	if windows.Handle(ret) != windows.S_OK {
		_ = pty.Close()
		return nil, fmt.Errorf("create pseudo console (%d): %w", int32(ret), err)
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
	if windows.Handle(ret) != windows.S_OK {
		return err
	}
	return nil
}
// closeConsoleNoLock closes the console handle, and sets it to
// windows.InvalidHandle. It must be called with p.closeMutex held.
func (p *ptyWindows) closeConsoleNoLock() error {
	// if we are running a command in the PTY, the corresponding *windowsProcess
	// may have already closed the PseudoConsole when the command exited, so that
	// output reads can get to EOF.  In that case, we don't need to close it
	// again here.
	if p.console != windows.InvalidHandle {
		// ClosePseudoConsole has no return value and typically the syscall
		// returns S_FALSE (a success value). We could ignore the return value
		// and error here but we handle anyway, it just in case.
		//
		// Note that ClosePseudoConsole is a blocking system call and may write
		// a final frame to the output buffer (p.outputWrite), so there must be
		// a consumer (p.outputRead) to ensure we don't block here indefinitely.
		//
		// https://docs.microsoft.com/en-us/windows/console/closepseudoconsole
		ret, _, err := procClosePseudoConsole.Call(uintptr(p.console))
		if winerrorFailed(ret) {
			return fmt.Errorf("close pseudo console (%d): %w", ret, err)
		}
		p.console = windows.InvalidHandle
	}
	return nil
}
func (p *ptyWindows) Close() error {
	p.closeMutex.Lock()
	defer p.closeMutex.Unlock()
	if p.closed {
		return nil
	}
	// Close the pseudo console, this will also terminate the process attached
	// to this pty. If it was created via Start(), this also unblocks close of
	// the readers below.
	err := p.closeConsoleNoLock()
	if err != nil {
		return err
	}
	// Only set closed after the console has been successfully closed.
	p.closed = true
	// Close the pipes ensuring that the writer is closed before the respective
	// reader, otherwise closing the reader may block indefinitely. Note that
	// outputWrite and inputRead are unset when we Start() a new process.
	if p.outputWrite != nil {
		_ = p.outputWrite.Close()
	}
	_ = p.outputRead.Close()
	_ = p.inputWrite.Close()
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
		err := p.pw.closeConsoleNoLock()
		// if we already have an error from the command, prefer that error
		// but if the command succeeded and closing the PseudoConsole fails
		// then record that error so that we have a chance to see it
		if err != nil && p.cmdErr == nil {
			p.cmdErr = err
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
func (p *windowsProcess) Signal(sig os.Signal) error {
	// Windows doesn't support signals.
	return p.Kill()
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
// winerrorFailed returns true if the syscall failed, this function
// assumes the return value is a 32-bit integer, like HRESULT.
//
// https://learn.microsoft.com/en-us/windows/win32/api/winerror/nf-winerror-failed
func winerrorFailed(r1 uintptr) bool {
	return int32(r1) < 0
}
