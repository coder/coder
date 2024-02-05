//go:build !linux
// +build !linux

package agentproc

import (
	"syscall"

	"golang.org/x/xerrors"
)

func NewSyscaller() Syscaller {
	return nopSyscaller{}
}

var errUnimplemented = xerrors.New("unimplemented")

type nopSyscaller struct{}

func (nopSyscaller) SetPriority(int32, int) error {
	return errUnimplemented
}

func (nopSyscaller) GetPriority(int32) (int, error) {
	return 0, errUnimplemented
}

func (nopSyscaller) Kill(int32, syscall.Signal) error {
	return errUnimplemented
}
