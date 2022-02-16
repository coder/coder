//go:build !windows
// +build !windows

package pty

import (
	"io"
	"os"

	"github.com/creack/pty"
)

func newPty() (Pty, error) {
	ptyFile, ttyFile, err := pty.Open()
	if err != nil {
		return nil, err
	}

	return &unixPty{
		pty: ptyFile,
		tty: ttyFile,
	}, nil
}

type unixPty struct {
	pty, tty *os.File
}

func (p *unixPty) InPipe() *os.File {
	return p.tty
}

func (p *unixPty) OutPipe() *os.File {
	return p.tty
}

func (p *unixPty) Reader() io.Reader {
	return p.pty
}

func (p *unixPty) WriteString(str string) (int, error) {
	return p.pty.WriteString(str)
}

func (p *unixPty) Resize(cols uint16, rows uint16) error {
	return pty.Setsize(p.tty, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

func (p *unixPty) Close() error {
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
