//go:build !linux
// +build !linux

package agentproc

import (
	"github.com/spf13/afero"
)

func (p *Process) Niceness(sc Syscaller) (int, error) {
	return 0, errUnimplemented
}

func (p *Process) SetNiceness(sc Syscaller, score int) error {
	return errUnimplemented
}

func (p *Process) Cmd() string {
	return ""
}

func List(fs afero.Fs, syscaller Syscaller) ([]*Process, error) {
	return nil, errUnimplemented
}
