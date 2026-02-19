package reaper

import (
	"os"

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

type options struct {
	ExecArgs     []string
	PIDs         reap.PidCh
	CatchSignals []os.Signal
	Logger       slog.Logger
}
