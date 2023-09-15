//go:build linux
// +build linux

package agentproc

import (
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

func NewSyscaller() Syscaller {
	return UnixSyscaller{}
}

type UnixSyscaller struct{}

func (UnixSyscaller) SetPriority(pid int32, nice int) error {
	err := unix.Setpriority(unix.PRIO_PROCESS, int(pid), nice)
	if err != nil {
		return xerrors.Errorf("set priority: %w", err)
	}
	return nil
}

func (UnixSyscaller) GetPriority(pid int32) (int, error) {
	nice, err := unix.Getpriority(0, int(pid))
	if err != nil {
		return 0, xerrors.Errorf("get priority: %w", err)
	}
	return nice, nil
}

func (UnixSyscaller) Kill(pid int32, sig syscall.Signal) error {
	err := syscall.Kill(int(pid), sig)
	if err != nil {
		return xerrors.Errorf("kill: %w", err)
	}

	return nil
}
