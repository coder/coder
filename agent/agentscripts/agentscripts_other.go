//go:build !windows

package agentscripts

import (
	"context"
	"os/exec"
	"syscall"

	"cdr.dev/slog"
)

func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}

func cmdCancel(ctx context.Context, logger slog.Logger, cmd *exec.Cmd) func() error {
	return func() error {
		logger.Debug(ctx, "cmdCancel: sending SIGHUP to process and children", slog.F("pid", cmd.Process.Pid))
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP)
	}
}
