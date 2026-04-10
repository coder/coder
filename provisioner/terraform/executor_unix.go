//go:build !windows

package terraform

import (
	"os"
	"syscall"

	"golang.org/x/xerrors"
)

// cmdSysProcAttr returns the SysProcAttr that places the child
// process in its own process group. This allows us to signal the
// entire tree (terraform + provider plugins) on cancellation
// instead of only the root process.
func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// signalProcessGroup sends sig to every process in the group
// rooted at the given pid. The negative pid is the POSIX
// convention for "all processes in this process group."
func signalProcessGroup(pid int, sig os.Signal) error {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return xerrors.Errorf("unsupported signal type: %T", sig)
	}
	return syscall.Kill(-pid, s)
}
