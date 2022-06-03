package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"cdr.dev/slog"
)

// agentPPROFStartOnUSR1 is no-op on Windows (no SIGUSR1 signal).
func agentPPROFStartOnUSR1(ctx context.Context, logger slog.Logger, pprofAddress string) (srvClose func()) {
	return func() {}
}
