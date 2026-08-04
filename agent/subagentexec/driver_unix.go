//go:build unix

package subagentexec

import (
	"os/exec"
	"syscall"

	"golang.org/x/xerrors"
)

// platformSupported reports that this build can isolate and signal a
// driver process group.
const platformSupported = true

// configureCommand puts the driver in its own process group so that
// stopping it also reaches the processes it spawned, which for a sandbox
// driver is where the child agent actually runs.
func configureCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// signalProcessGroup signals the whole group the driver leads. Setpgid
// makes the group ID equal to the driver's PID, so the negated PID
// addresses the group.
func signalProcessGroup(cmd *exec.Cmd, sig procSignal) error {
	if cmd.Process == nil {
		return xerrors.New("driver process was never started")
	}
	var signal syscall.Signal
	switch sig {
	case signalTerminate:
		signal = syscall.SIGTERM
	case signalKill:
		signal = syscall.SIGKILL
	default:
		return xerrors.Errorf("unknown signal %d", int(sig))
	}
	if err := syscall.Kill(-cmd.Process.Pid, signal); err != nil {
		return xerrors.Errorf("signal driver process group %d with %s: %w", cmd.Process.Pid, signal, err)
	}
	return nil
}
