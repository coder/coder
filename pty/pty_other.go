//go:build !windows
// +build !windows

package pty

import (
	"io"
	"os"

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
	pty, tty *os.File
}

func (p *otherPty) Input() io.ReadWriter {
	return p.pty
}

func (p *otherPty) Output() io.ReadWriter {
	return p.pty
}

func (p *otherPty) WriteString(str string) (int, error) {
	return p.pty.WriteString(str)
}

func (p *otherPty) Resize(cols uint16, rows uint16) error {
	return pty.Setsize(p.tty, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

func (p *otherPty) Close() error {
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
