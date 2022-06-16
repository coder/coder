package reaper

import (
	"fmt"
	"os"
	"syscall"

	"github.com/hashicorp/go-reap"
	"golang.org/x/xerrors"
)

// agentEnvMark is a simple environment variable that we use as a marker
// to indicated that the process is a child as opposed to the reaper.
// Since we are forkexec'ing we need to be able to differentiate between
// the two to avoid fork bombing ourselves.
const agentEnvMark = "CODER_DO_NOT_REAP"

// IsChild returns true if we're the forked process.
func IsChild() bool {
	return os.Getenv(agentEnvMark) != ""
}

// IsInitProcess returns true if the current process's PID is 1.
func IsInitProcess() bool {
	return os.Getpid() == 1
}

// ForkReap spawns a goroutine that reaps children. In order to avoid
// complications with spawning `exec.Commands` in the same process that
// is reaping, we forkexec a child process. This prevents a race between
// the reaper and an exec.Command waiting for its process to complete.
// The provided 'pids' channel may be nil if the caller does not care about the
// reaped children PIDs.
func ForkReap(pids reap.PidCh) error {
	// Check if the process is the parent or the child.
	// If it's the child we want to skip attempting to reap.
	if IsChild() {
		return nil
	}

	go reap.ReapChildren(pids, nil, nil, nil)

	args := os.Args
	// This is simply done to help identify the real agent process
	// when viewing in something like 'ps'.
	args = append(args, "#Agent")

	pwd, err := os.Getwd()
	if err != nil {
		return xerrors.Errorf("get wd: %w", err)
	}

	pattrs := &syscall.ProcAttr{
		Dir: pwd,
		// Add our marker for identifying the child process.
		Env: append(os.Environ(), fmt.Sprintf("%s=true", agentEnvMark)),
		Sys: &syscall.SysProcAttr{
			Setsid: true,
		},
		Files: []uintptr{
			uintptr(syscall.Stdin),
			uintptr(syscall.Stdout),
			uintptr(syscall.Stderr),
		},
	}

	//#nosec G204
	pid, _ := syscall.ForkExec(args[0], args, pattrs)

	var wstatus syscall.WaitStatus
	_, err = syscall.Wait4(pid, &wstatus, 0, nil)
	for xerrors.Is(err, syscall.EINTR) {
		_, err = syscall.Wait4(pid, &wstatus, 0, nil)
	}

	return nil
}
