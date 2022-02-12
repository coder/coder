//go:build windows
// +build windows

package pty

import (
	"io"
	"os"

	 "golang.org/x/sys/windows"

	 "github.com/coder/coder/expect/conpty"
)

func newPty() (Pty, error) {
	// We use the CreatePseudoConsole API which was introduced in build 17763
	vsn := windows.RtlGetVersion()
	if vsn.MajorVersion < 10 ||
		vsn.BuildNumber < 17763 {
		return pipePty()
	 }

	return conpty.New(80, 80)
}

func pipePty() (Pty, error) {
	inputR, inputW, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	outputR, outputW, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	return &pipePtyVal{
		inputR,
		inputW,
		outputR,
		outputW,
	}, nil
}

type pipePtyVal struct {
	inputR, inputW *os.File
	outputR, outputW *os.File
}

func (p *pipePtyVal) InPipe() *os.File {
	return p.inputR
}

func (p *pipePtyVal) OutPipe() *os.File {
	return p.outputW
}

func (p *pipePtyVal) Reader() io.Reader {
	return p.outputR
}

func (p *pipePtyVal) WriteString(str string) (int, error) {
	return p.inputW.WriteString(str)
}

func (p *pipePtyVal) Resize(uint16, uint16) error {
	return nil
}

func (p *pipePtyVal) Close() error {
	p.inputW.Close()
	p.inputR.Close()
	p.outputW.Close()
	p.outputR.Close()
	return nil
}
