package reaper

import (
	"os"
	"time"

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

// WithMaxRestarts sets the maximum number of times the child process
// will be restarted after being killed by SIGKILL within the restart
// window. Default is 5.
func WithMaxRestarts(n int) Option {
	return func(o *options) {
		o.MaxRestarts = n
	}
}

// WithRestartWindow sets the sliding time window within which restart
// attempts are counted. If the max restarts are exhausted within this
// window, the reaper gives up. Default is 10 minutes.
func WithRestartWindow(d time.Duration) Option {
	return func(o *options) {
		o.RestartWindow = d
	}
}

// WithRestartBaseDelay sets the initial backoff delay before restarting
// the child process. The delay doubles on each subsequent restart.
// Default is 1 second.
func WithRestartBaseDelay(d time.Duration) Option {
	return func(o *options) {
		o.RestartBaseDelay = d
	}
}

// WithRestartMaxDelay sets the maximum backoff delay before restarting
// the child process. Default is 60 seconds.
func WithRestartMaxDelay(d time.Duration) Option {
	return func(o *options) {
		o.RestartMaxDelay = d
	}
}

type options struct {
	ExecArgs     []string
	PIDs         reap.PidCh
	CatchSignals []os.Signal
	Logger       slog.Logger

	// Restart options for crash-loop recovery (e.g. OOM kills).
	MaxRestarts      int
	RestartWindow    time.Duration
	RestartBaseDelay time.Duration
	RestartMaxDelay  time.Duration
}
