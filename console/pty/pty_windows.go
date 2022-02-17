//go:build windows
// +build windows

package pty

import (
	"io"
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
func newPty() (Pty, error) {
	// We use the CreatePseudoConsole API which was introduced in build 17763
	vsn := windows.RtlGetVersion()
	if vsn.MajorVersion < 10 ||
		vsn.BuildNumber < 17763 {
		// If the CreatePseudoConsole API is not available, we fall back to a simpler
		// implementation that doesn't create an actual PTY - just uses os.Pipe
		return pipePty()
	}

	ptyWindows := &ptyWindows{}

	// Create the stdin pipe
	if err := windows.CreatePipe(&ptyWindows.inputReadSide, &ptyWindows.inputWriteSide, nil, 0); err != nil {
		return nil, err
	}

	// Create the stdout pipe
	if err := windows.CreatePipe(&ptyWindows.outputReadSide, &ptyWindows.outputWriteSide, nil, 0); err != nil {
		return nil, err
	}

	consoleSize := uintptr((int32(80) << 16) | int32(80))
	ret, _, err := procCreatePseudoConsole.Call(
		consoleSize,
		uintptr(ptyWindows.inputReadSide),
		uintptr(ptyWindows.outputWriteSide),
		0,
		uintptr(unsafe.Pointer(&ptyWindows.console)),
	)
	if ret != 0 {
		return nil, xerrors.Errorf("create pseudo console (%d): %w", ret, err)
	}

	ptyWindows.outputWriteSideFile = os.NewFile(uintptr(ptyWindows.outputWriteSide), "|0")
	ptyWindows.outputReadSideFile = os.NewFile(uintptr(ptyWindows.outputReadSide), "|1")
	ptyWindows.inputReadSideFile = os.NewFile(uintptr(ptyWindows.inputReadSide), "|2")
	ptyWindows.inputWriteSideFile = os.NewFile(uintptr(ptyWindows.inputWriteSide), "|3")
	ptyWindows.closed = false

	return ptyWindows, nil
}

type ptyWindows struct {
	console windows.Handle

	outputWriteSide windows.Handle
	outputReadSide  windows.Handle
	inputReadSide   windows.Handle
	inputWriteSide  windows.Handle

	outputWriteSideFile *os.File
	outputReadSideFile  *os.File
	inputReadSideFile   *os.File
	inputWriteSideFile  *os.File

	closeMutex sync.Mutex
	closed     bool
}

func (p *ptyWindows) Input() io.ReadWriter {
	return readWriter{
		Writer: p.inputWriteSideFile,
		Reader: p.inputReadSideFile,
	}
}

func (p *ptyWindows) Output() io.ReadWriter {
	return readWriter{
		Writer: p.outputWriteSideFile,
		Reader: p.outputReadSideFile,
	}
}

func (p *ptyWindows) Resize(cols uint16, rows uint16) error {
	ret, _, err := procResizePseudoConsole.Call(uintptr(p.console), uintptr(cols)+(uintptr(rows)<<16))
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

	ret, _, err := procClosePseudoConsole.Call(uintptr(p.console))
	if ret != 0 {
		return xerrors.Errorf("close pseudo console: %w", err)
	}
	_ = p.outputWriteSideFile.Close()
	_ = p.outputReadSideFile.Close()
	_ = p.inputReadSideFile.Close()
	_ = p.inputWriteSideFile.Close()
	return nil
}
