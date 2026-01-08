package agentssh

import (
	"context"
	"os"
	"syscall"

	"cdr.dev/slog/v3"
)

func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func cmdCancel(logger slog.Logger, p *os.Process) error {
	logger.Debug(context.Background(), "cmdCancel: killing process", slog.F("pid", p.Pid))
	// Windows doesn't support sending signals to process groups, so we
	// have to kill the process directly. In the future, we may want to
	// implement a more sophisticated solution for process groups on
	// Windows, but for now, this is a simple way to ensure that the
	// process is terminated when the context is cancelled.
	return p.Kill()
}
