//go:build !linux
// +build !linux

package agentproc

import "syscall"

func NewSyscaller() Syscaller {
	return nopSyscaller{}
}

type nopSyscaller struct{}

func (nopSyscaller) SetPriority(pid int32, priority int) error {
	return nil
}

func (nopSyscaller) GetPriority(pid int32) (int, error) {
	return 0, nil
}

func (nopSyscaller) Kill(pid int32, sig syscall.Signal) error {
	return nil
}
