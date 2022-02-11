//go:build windows
// +build windows

package pty

import (
	"os"

	"golang.org/x/sys/windows"

	"github.com/hashicorp/waypoint-plugin-sdk/internal/pkg/conpty"
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
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	return &pipePtyVal{r: r, w: w}, nil
}

type pipePtyVal struct {
	r, w *os.File
}

func (p *pipePtyVal) InPipe() *os.File {
	return p.w
}

func (p *pipePtyVal) OutPipe() *os.File {
	return p.r
}

func (p *pipePtyVal) WriteString(string) (int, error) {
	return p.w.WriteString(string)
}

func (p *pipePtyVal) Resize(uint16, uint16) error {
	return nil
}

func (p *pipePtyVal) Close() error {
	p.w.Close()
	p.r.Close()
	return nil
}
