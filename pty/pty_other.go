//go:build !windows
// +build !windows

package pty

import (
	"io"
	"os"
	"sync"

	"github.com/creack/pty"
)

func newPty() (PTY, error) {
	ptyFile, ttyFile, err := pty.Open()
	if err != nil {
		return nil, err
	}

	return &otherPty{
		pty: ptyFile,
		tty: ttyFile,
	}, nil
}

type otherPty struct {
	mutex    sync.Mutex
	pty, tty *os.File
}

func (p *otherPty) Input() io.ReadWriter {
	return readWriter{
		Reader: p.tty,
		Writer: p.pty,
	}
}

func (p *otherPty) Output() io.ReadWriter {
	return readWriter{
		Reader: p.pty,
		Writer: p.tty,
	}
}

func (p *otherPty) Resize(height uint16, width uint16) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return pty.Setsize(p.pty, &pty.Winsize{
		Rows: width,
		Cols: height,
	})
}

func (p *otherPty) Close() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	err := p.pty.Close()
	if err != nil {
		return err
	}

	err = p.tty.Close()
	if err != nil {
		return err
	}
	return nil
}
