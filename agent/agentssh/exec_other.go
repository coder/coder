//go:build !windows

package agentssh

import (
	"context"
	"os"
	"syscall"

	"cdr.dev/slog/v3"
)

func cmdSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}

func cmdCancel(logger slog.Logger, p *os.Process) error {
	logger.Debug(context.Background(), "cmdCancel: sending SIGHUP to process and children", slog.F("pid", p.Pid))
	return syscall.Kill(-p.Pid, syscall.SIGHUP)
}
