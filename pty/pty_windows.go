//go:build windows
// +build windows

package pty

import (
	"os"
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
func newPty() (PTY, error) {
	// We use the CreatePseudoConsole API which was introduced in build 17763
	vsn := windows.RtlGetVersion()
	if vsn.MajorVersion < 10 ||
		vsn.BuildNumber < 17763 {
		// If the CreatePseudoConsole API is not available, we fall back to a simpler
		// implementation that doesn't create an actual PTY - just uses os.Pipe
		return nil, xerrors.Errorf("pty not supported")
	}

	ptyWindows := &ptyWindows{}

	var err error
	ptyWindows.inputRead, ptyWindows.inputWrite, err = os.Pipe()
	if err != nil {
		return nil, err
	}
	ptyWindows.outputRead, ptyWindows.outputWrite, err = os.Pipe()

	consoleSize := uintptr(80) + (uintptr(80) << 16)
	ret, _, err := procCreatePseudoConsole.Call(
		consoleSize,
		uintptr(ptyWindows.inputRead.Fd()),
		uintptr(ptyWindows.outputWrite.Fd()),
		0,
		uintptr(unsafe.Pointer(&ptyWindows.console)),
	)
	if int32(ret) < 0 {
		return nil, xerrors.Errorf("create pseudo console (%d): %w", int32(ret), err)
	}
	return ptyWindows, nil
}

type ptyWindows struct {
	console windows.Handle

	outputWrite *os.File
	outputRead  *os.File
	inputWrite  *os.File
	inputRead   *os.File

	closeMutex sync.Mutex
	closed     bool
}

func (p *ptyWindows) Output() ReadWriter {
	return ReadWriter{
		Reader: p.outputRead,
		Writer: p.outputWrite,
	}
}

func (p *ptyWindows) Input() ReadWriter {
	return ReadWriter{
		Reader: p.inputRead,
		Writer: p.inputWrite,
	}
}

func (p *ptyWindows) Resize(height uint16, width uint16) error {
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
	_ = p.outputWrite.Close()
	_ = p.outputRead.Close()
	_ = p.inputWrite.Close()
	_ = p.inputRead.Close()

	ret, _, err := procClosePseudoConsole.Call(uintptr(p.console))
	if ret < 0 {
		return xerrors.Errorf("close pseudo console: %w", err)
	}

	return nil
}
