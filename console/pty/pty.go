package pty

import (
	"io"
	"os"
)

// Pty is the minimal pseudo-tty interface we require.
type Pty interface {
	Input() io.ReadWriter
	Output() io.ReadWriter
	Resize(cols uint16, rows uint16) error
	Close() error
}

// New creates a new Pty.
func New() (Pty, error) {
	return newPty()
}

func pipePty() (Pty, error) {
	inFilePipeSide, inFileOurSide, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	outFileOurSide, outFilePipeSide, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	return &pipePtyVal{
		inFilePipeSide,
		inFileOurSide,
		outFileOurSide,
		outFilePipeSide,
	}, nil
}

type pipePtyVal struct {
	inFilePipeSide, inFileOurSide   *os.File
	outFileOurSide, outFilePipeSide *os.File
}

func (p *pipePtyVal) Output() io.ReadWriter {
	return readWriter{
		Reader: p.outFilePipeSide,
		Writer: p.outFileOurSide,
	}
}

func (p *pipePtyVal) Input() io.ReadWriter {
	return readWriter{
		Reader: p.inFilePipeSide,
		Writer: p.inFileOurSide,
	}
}

func (p *pipePtyVal) WriteString(str string) (int, error) {
	return p.inFileOurSide.WriteString(str)
}

func (p *pipePtyVal) Resize(uint16, uint16) error {
	return nil
}

func (p *pipePtyVal) Close() error {
	p.inFileOurSide.Close()
	p.inFilePipeSide.Close()
	p.outFilePipeSide.Close()
	p.outFileOurSide.Close()
	return nil
}

type readWriter struct {
	io.Reader
	io.Writer
}
