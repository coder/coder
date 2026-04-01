package reaper

import (
	"os"
	"sync"

	"github.com/hashicorp/go-reap"

	"cdr.dev/slog/v3"
)

type Option func(o *options)

// WithExecArgs specifies the exec arguments for the fork exec call.
// By default the same arguments as the parent are used as dictated by
// os.Args. Since ForkReap calls a fork-exec it is the responsibility of
// the caller to avoid fork-bombing oneself.
func WithExecArgs(args ...string) Option {
	return func(o *options) {
		o.ExecArgs = args
	}
}

// WithPIDCallback sets the channel that reaped child process PIDs are pushed
// onto.
func WithPIDCallback(ch reap.PidCh) Option {
	return func(o *options) {
		o.PIDs = ch
	}
}

// WithCatchSignals sets the signals that are caught and forwarded to the
// child process. By default no signals are forwarded.
func WithCatchSignals(sigs ...os.Signal) Option {
	return func(o *options) {
		o.CatchSignals = sigs
	}
}

func WithLogger(logger slog.Logger) Option {
	return func(o *options) {
		o.Logger = logger
	}
}

// WithReaperStop sets a channel that, when closed, stops the reaper
// goroutine. Callers that invoke ForkReap more than once in the
// same process (e.g. tests) should use this to prevent goroutine
// accumulation.
func WithReaperStop(ch chan struct{}) Option {
	return func(o *options) {
		o.ReaperStop = ch
	}
}

// WithReaperStopped sets a channel that is closed after the
// reaper goroutine has fully exited.
func WithReaperStopped(ch chan struct{}) Option {
	return func(o *options) {
		o.ReaperStopped = ch
	}
}

// WithReapLock sets a mutex shared between the reaper and Wait4.
// The reaper holds the write lock while reaping, and ForkReap
// holds the read lock during Wait4, preventing the reaper from
// stealing the child's exit status. This is only needed for
// tests with instant-exit children where the race window is
// large.
func WithReapLock(mu *sync.RWMutex) Option {
	return func(o *options) {
		o.ReapLock = mu
	}
}

type options struct {
	ExecArgs      []string
	PIDs          reap.PidCh
	CatchSignals  []os.Signal
	Logger        slog.Logger
	ReaperStop    chan struct{}
	ReaperStopped chan struct{}
	ReapLock      *sync.RWMutex
}
