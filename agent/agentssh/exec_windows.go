package agentssh

import (
	"context"
	"os/exec"
	"syscall"

	"cdr.dev/slog"
)

func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func cmdCancel(ctx context.Context, logger slog.Logger, cmd *exec.Cmd) func() error {
	return func() error {
		logger.Debug(ctx, "cmdCancel: killing process", slog.F("pid", cmd.Process.Pid))
		// Windows doesn't support sending signals to process groups, so we
		// have to kill the process directly. In the future, we may want to
		// implement a more sophisticated solution for process groups on
		// Windows, but for now, this is a simple way to ensure that the
		// process is terminated when the context is cancelled.
		return cmd.Process.Kill()
	}
}
