//go:build linux

package reaper

import (
	"os"
	"syscall"

	"github.com/hashicorp/go-reap"
	"golang.org/x/xerrors"
)

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
func ForkReap(opt ...Option) error {
	opts := &options{
		ExecArgs: os.Args,
	}

	for _, o := range opt {
		o(opts)
	}

	go reap.ReapChildren(opts.PIDs, nil, nil, nil)

	pwd, err := os.Getwd()
	if err != nil {
		return xerrors.Errorf("get wd: %w", err)
	}

	pattrs := &syscall.ProcAttr{
		Dir: pwd,
		Env: os.Environ(),
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
	pid, _ := syscall.ForkExec(opts.ExecArgs[0], opts.ExecArgs, pattrs)

	var wstatus syscall.WaitStatus
	_, err = syscall.Wait4(pid, &wstatus, 0, nil)
	for xerrors.Is(err, syscall.EINTR) {
		_, err = syscall.Wait4(pid, &wstatus, 0, nil)
	}

	return nil
}
