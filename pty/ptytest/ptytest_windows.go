//go:build windows

package ptytest

import (
	"os"
	"sync"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/pty"
)

// testPTY is a pipe-based PTY implementation for in-process CLI testing on
// Windows. ConPTY requires an attached process to function correctly - without
// one, the pipe handles become invalid intermittently. This implementation
// avoids ConPTY entirely for the ptytest.New() + Attach() use case.
type testPTY struct {
	inputReader  *os.File
	inputWriter  *os.File
	outputReader *os.File
	outputWriter *os.File

	closeMutex sync.Mutex
	closed     bool
}

func newTestPTY(_ ...pty.Option) (pty.PTY, error) {
	p := &testPTY{}

	var err error
	p.inputReader, p.inputWriter, err = os.Pipe()
	if err != nil {
		return nil, xerrors.Errorf("create input pipe: %w", err)
	}
	p.outputReader, p.outputWriter, err = os.Pipe()
	if err != nil {
		_ = p.inputReader.Close()
		_ = p.inputWriter.Close()
		return nil, xerrors.Errorf("create output pipe: %w", err)
	}

	return p, nil
}

func (*testPTY) Name() string {
	return ""
}

func (p *testPTY) Input() pty.ReadWriter {
	return pty.ReadWriter{
		Reader: p.inputReader,
		Writer: p.inputWriter,
	}
}

func (p *testPTY) Output() pty.ReadWriter {
	return pty.ReadWriter{
		Reader: p.outputReader,
		Writer: p.outputWriter,
	}
}

func (*testPTY) Resize(uint16, uint16) error {
	return nil
}

func (p *testPTY) Close() error {
	p.closeMutex.Lock()
	defer p.closeMutex.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true

	var firstErr error
	if err := p.outputWriter.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := p.outputReader.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := p.inputWriter.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := p.inputReader.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}
