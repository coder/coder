//go:build !linux
// +build !linux

package agentproc

import (
	"github.com/spf13/afero"
)

func (p *Process) SetOOMAdj(score int) error {
	return errUnimplimented
}

func (p *Process) Niceness(sc Syscaller) (int, error) {
	return 0, errUnimplimented
}

func (p *Process) SetNiceness(sc Syscaller, score int) error {
	return errUnimplimented
}

func (p *Process) Name() string {
	return ""
}

func List(fs afero.Fs, syscaller Syscaller) ([]*Process, error) {
	return nil, errUnimplimented
}
