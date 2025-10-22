package agentscripts

import (
	"context"
	"os"
	"os/exec"
	"syscall"

	"cdr.dev/slog"
)

func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func cmdCancel(ctx context.Context, logger slog.Logger, cmd *exec.Cmd) func() error {
	return func() error {
		logger.Debug(ctx, "cmdCancel: sending interrupt to process", slog.F("pid", cmd.Process.Pid))
		return cmd.Process.Signal(os.Interrupt)
	}
}
