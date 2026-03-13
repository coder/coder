//go:build linux

package reaper

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/hashicorp/go-reap"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// IsInitProcess returns true if the current process's PID is 1.
func IsInitProcess() bool {
	return os.Getpid() == 1
}

func catchSignals(logger slog.Logger, pid int, sc <-chan os.Signal) {
	for {
		s := <-sc
		sig, ok := s.(syscall.Signal)
		if ok {
			logger.Info(context.Background(), "reaper caught signal, killing child process",
				slog.F("signal", sig.String()),
				slog.F("child_pid", pid),
			)
			_ = syscall.Kill(pid, sig)
		}
	}
}

// ForkReap spawns a goroutine that reaps children. In order to avoid
// complications with spawning `exec.Commands` in the same process that
// is reaping, we forkexec a child process. This prevents a race between
// the reaper and an exec.Command waiting for its process to complete.
// The provided 'pids' channel may be nil if the caller does not care about the
// reaped children PIDs.
//
// Returns the child's exit code (using 128+signal for signal termination)
// and any error from Wait4.
func ForkReap(opt ...Option) (int, error) {
	opts := &options{
		ExecArgs: os.Args,
	}

	for _, o := range opt {
		o(opts)
	}

	// Use the reapLock to prevent the reaper's Wait4(-1) from
	// stealing the direct child's exit status. The reaper takes
	// a write lock; we hold a read lock during our own Wait4.
	var reapLock sync.RWMutex
	reapLock.RLock()

	go reap.ReapChildren(opts.PIDs, nil, opts.Done, &reapLock)

	pwd, err := os.Getwd()
	if err != nil {
		reapLock.RUnlock()
		return 1, xerrors.Errorf("get wd: %w", err)
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
	pid, err := syscall.ForkExec(opts.ExecArgs[0], opts.ExecArgs, pattrs)
	if err != nil {
		reapLock.RUnlock()
		return 1, xerrors.Errorf("fork exec: %w", err)
	}

	// Register the signal handler before spawning the goroutine
	// so it is active by the time the child process starts. This
	// avoids a race where a signal arrives before the goroutine
	// has called signal.Notify.
	if len(opts.CatchSignals) > 0 {
		sc := make(chan os.Signal, 1)
		signal.Notify(sc, opts.CatchSignals...)
		go catchSignals(opts.Logger, pid, sc)
	}

	var wstatus syscall.WaitStatus
	_, err = syscall.Wait4(pid, &wstatus, 0, nil)
	for xerrors.Is(err, syscall.EINTR) {
		_, err = syscall.Wait4(pid, &wstatus, 0, nil)
	}
	reapLock.RUnlock()

	// Convert wait status to exit code using standard Unix conventions:
	// - Normal exit: use the exit code
	// - Signal termination: use 128 + signal number
	var exitCode int
	switch {
	case wstatus.Exited():
		exitCode = wstatus.ExitStatus()
	case wstatus.Signaled():
		exitCode = 128 + int(wstatus.Signal())
	default:
		exitCode = 1
	}
	return exitCode, err
}
