package pty

import (
	"os/exec"
)

// StartOption represents a configuration option passed to Start.
type StartOption func(*startOptions)

type startOptions struct {
	ptyOpts []Option
}

// WithPTYOption applies the given options to the underlying PTY.
func WithPTYOption(opts ...Option) StartOption {
	return func(o *startOptions) {
		o.ptyOpts = append(o.ptyOpts, opts...)
	}
}

// Start the command in a TTY.  The calling code must not use cmd after passing it to the PTY, and
// instead rely on the returned Process to manage the command/process.
func Start(cmd *exec.Cmd, opt ...StartOption) (PTY, Process, error) {
	return startPty(cmd, opt...)
}
