//go:build windows
// +build windows

package pty

import (
	"io"
	"os"

	"golang.org/x/sys/windows"

	"github.com/coder/coder/console/conpty"
)

func newPty() (Pty, error) {
	// We use the CreatePseudoConsole API which was introduced in build 17763
	vsn := windows.RtlGetVersion()
	if vsn.MajorVersion < 10 ||
		vsn.BuildNumber < 17763 {
		// If the CreatePseudoConsole API is not available, we fall back to a simpler
		// implementation that doesn't create an actual PTY - just uses os.Pipe
		return pipePty()
	}

	return conpty.New(80, 80)
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

func (p *pipePtyVal) InPipe() *os.File {
	return p.inFilePipeSide
}

func (p *pipePtyVal) OutPipe() *os.File {
	return p.outFilePipeSide
}

func (p *pipePtyVal) Reader() io.Reader {
	return p.outFileOurSide
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
