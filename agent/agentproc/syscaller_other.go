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

func (nopSyscaller) SetPriority(pid int32, priority int) error {
	return errUnimplimented
}

func (nopSyscaller) GetPriority(pid int32) (int, error) {
	return 0, errUnimplimented
}

func (nopSyscaller) Kill(pid int32, sig syscall.Signal) error {
	return errUnimplimented
}
