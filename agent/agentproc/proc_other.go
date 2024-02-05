//go:build !linux
// +build !linux

package agentproc

import (
	"github.com/spf13/afero"
)

func (*Process) Niceness(Syscaller) (int, error) {
	return 0, errUnimplemented
}

func (*Process) SetNiceness(Syscaller, int) error {
	return errUnimplemented
}

func (*Process) Cmd() string {
	return ""
}

func List(afero.Fs, Syscaller) ([]*Process, error) {
	return nil, errUnimplemented
}
