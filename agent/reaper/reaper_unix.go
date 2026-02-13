//go:build linux

package reaper

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/go-reap"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
	defaultMaxRestarts      = 5
	defaultRestartWindow    = 10 * time.Minute
	defaultRestartBaseDelay = 1 * time.Second
	defaultRestartMaxDelay  = 60 * time.Second

	// StartCountFile tracks how many times the agent process has
	// started. A value > 1 indicates the agent was restarted
	// (e.g. after an OOM kill). The file is written by the reaper
	// in PID 1 mode and by the agent itself in systemd mode. It
	// is deleted on graceful shutdown.
	StartCountFile = "/tmp/coder-agent-start-count.txt"
	// KillSignalFile records the signal that terminated the
	// previous agent process (e.g. "SIGKILL"). Written by the
	// reaper after wait4 in the PID 1 path, or by systemd's
	// ExecStopPost in the supervised path. Deleted on graceful
	// shutdown.
	KillSignalFile = "/tmp/coder-agent-kill-signal.txt"
)

// IsInitProcess returns true if the current process's PID is 1.
func IsInitProcess() bool {
	return os.Getpid() == 1
}

// catchSignalsWithStop catches the given signals and forwards them to
// the child process. On the first signal received, it closes the
// stopping channel to indicate that the reaper should not restart the
// child. Subsequent signals are still forwarded.
func catchSignalsWithStop(logger slog.Logger, pid int, sigs []os.Signal, stopping chan struct{}, once *sync.Once) {
	if len(sigs) == 0 {
		return
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, sigs...)
	defer signal.Stop(sc)

	logger.Info(context.Background(), "reaper catching signals",
		slog.F("signals", sigs),
		slog.F("child_pid", pid),
	)

	for s := range sc {
		sig, ok := s.(syscall.Signal)
		if !ok {
			continue
		}
		// Signal that we're intentionally stopping — suppress
		// restart after the child exits.
		once.Do(func() { close(stopping) })
		logger.Info(context.Background(), "reaper caught signal, forwarding to child",
			slog.F("signal", sig.String()),
			slog.F("child_pid", pid),
		)
		_ = syscall.Kill(pid, sig)
	}
}

// ForkReap spawns a goroutine that reaps children. In order to avoid
// complications with spawning `exec.Commands` in the same process that
// is reaping, we forkexec a child process. This prevents a race between
// the reaper and an exec.Command waiting for its process to complete.
// The provided 'pids' channel may be nil if the caller does not care
// about the reaped children PIDs.
//
// If the child process is killed by SIGKILL (e.g. by the OOM killer),
// ForkReap will restart it with exponential backoff, up to MaxRestarts
// times within RestartWindow. If the reaper receives a stop signal
// (via CatchSignals), it will not restart the child after it exits.
//
// Returns the child's exit code (using 128+signal for signal
// termination) and any error from Wait4.
func ForkReap(opt ...Option) (int, error) {
	opts := &options{
		ExecArgs:         os.Args,
		MaxRestarts:      defaultMaxRestarts,
		RestartWindow:    defaultRestartWindow,
		RestartBaseDelay: defaultRestartBaseDelay,
		RestartMaxDelay:  defaultRestartMaxDelay,
	}

	for _, o := range opt {
		o(opts)
	}

	go reap.ReapChildren(opts.PIDs, nil, nil, nil)

	pwd, err := os.Getwd()
	if err != nil {
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

	// Track whether we've been told to stop via a caught signal.
	stopping := make(chan struct{})
	var stoppingOnce sync.Once

	var restartCount int
	var restartTimes []time.Time

	for {
		// Write the start count before forking so the child can
		// detect restarts. Start count = restartCount + 1 (first
		// start is 1, first restart is 2, etc.).
		if err := WriteStartCount(restartCount + 1); err != nil {
			opts.Logger.Error(context.Background(), "failed to write start count file", slog.Error(err))
		}

		//#nosec G204
		pid, err := syscall.ForkExec(opts.ExecArgs[0], opts.ExecArgs, pattrs)
		if err != nil {
			return 1, xerrors.Errorf("fork exec: %w", err)
		}

		go catchSignalsWithStop(opts.Logger, pid, opts.CatchSignals, stopping, &stoppingOnce)

		var wstatus syscall.WaitStatus
		_, err = syscall.Wait4(pid, &wstatus, 0, nil)
		for xerrors.Is(err, syscall.EINTR) {
			_, err = syscall.Wait4(pid, &wstatus, 0, nil)
		}

		exitCode := convertExitCode(wstatus)

		if !shouldRestart(wstatus, stopping, restartTimes, opts) {
			return exitCode, err
		}

		// Record the signal that killed the child so the next
		// instance can report it to coderd.
		if wstatus.Signaled() {
			if err := WriteKillSignal(wstatus.Signal().String()); err != nil {
				opts.Logger.Error(context.Background(), "failed to write kill signal file", slog.Error(err))
			}
		}

		restartCount++
		restartTimes = append(restartTimes, time.Now())
		delay := backoffDelay(restartCount, opts.RestartBaseDelay, opts.RestartMaxDelay)
		opts.Logger.Warn(context.Background(), "child process killed, restarting",
			slog.F("restart_count", restartCount),
			slog.F("signal", wstatus.Signal()),
			slog.F("delay", delay),
		)

		select {
		case <-time.After(delay):
			// Continue to restart.
		case <-stopping:
			return exitCode, err
		}
	}
}

// shouldRestart determines whether the child process should be
// restarted based on its exit status, whether we're stopping, and
// how many recent restarts have occurred.
func shouldRestart(wstatus syscall.WaitStatus, stopping <-chan struct{}, restartTimes []time.Time, opts *options) bool {
	// Don't restart if we've been told to stop.
	select {
	case <-stopping:
		return false
	default:
	}

	// Only restart on SIGKILL (signal 9), which is what the OOM
	// killer sends. Other signals (SIGTERM, SIGINT, etc.) indicate
	// intentional termination.
	if !wstatus.Signaled() || wstatus.Signal() != syscall.SIGKILL {
		return false
	}

	// Count restarts within the sliding window.
	cutoff := time.Now().Add(-opts.RestartWindow)
	recentCount := 0
	for _, t := range restartTimes {
		if t.After(cutoff) {
			recentCount++
		}
	}
	return recentCount < opts.MaxRestarts
}

// convertExitCode converts a wait status to an exit code using
// standard Unix conventions.
func convertExitCode(wstatus syscall.WaitStatus) int {
	switch {
	case wstatus.Exited():
		return wstatus.ExitStatus()
	case wstatus.Signaled():
		return 128 + int(wstatus.Signal())
	default:
		return 1
	}
}

// backoffDelay computes an exponential backoff delay with jitter.
// The delay doubles on each attempt, capped at maxDelay, with
// 0-25% jitter added to prevent thundering herd.
func backoffDelay(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	// Cap the shift amount to prevent overflow. With a 1s base
	// delay, shift > 60 would overflow time.Duration (int64).
	shift := attempt - 1
	if shift > 60 {
		shift = 60
	}
	// #nosec G115 - shift is capped above, so this is safe.
	delay := baseDelay * time.Duration(1<<uint(shift))
	if delay > maxDelay {
		delay = maxDelay
	}
	// Add 0-25% jitter.
	if delay > 0 {
		//nolint:gosec // Jitter doesn't need cryptographic randomness.
		jitter := time.Duration(rand.Int63n(int64(delay / 4)))
		delay += jitter
	}
	return delay
}

// WriteStartCount writes the start count to the well-known file.
// The reaper calls this before forking each child so the agent
// can detect it has been restarted (start count > 1).
func WriteStartCount(count int) error {
	if err := os.WriteFile(StartCountFile, []byte(fmt.Sprintf("%d", count)), 0o644); err != nil {
		return xerrors.Errorf("write start count file: %w", err)
	}
	return nil
}

// WriteKillSignal writes the signal name (e.g. "killed") to the
// well-known file so the agent can report it to coderd.
func WriteKillSignal(sig string) error {
	if err := os.WriteFile(KillSignalFile, []byte(sig), 0o644); err != nil {
		return xerrors.Errorf("write kill signal file: %w", err)
	}
	return nil
}

// ReadKillSignal reads the kill signal from the well-known file.
// Returns an empty string if the file doesn't exist.
func ReadKillSignal() string {
	data, err := os.ReadFile(KillSignalFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ClearRestartState deletes the start count and kill signal files.
// This should be called on graceful shutdown so the next start
// begins fresh.
func ClearRestartState() {
	_ = os.Remove(StartCountFile)
	_ = os.Remove(KillSignalFile)
}
