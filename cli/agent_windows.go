package cli

import (
	"context"

	"cdr.dev/slog"
)

// agentStartPPROFOnUSR1 is no-op on Windows (no SIGUSR1 signal).
func agentStartPPROFOnUSR1(ctx context.Context, logger slog.Logger, pprofAddress string) (srvClose func()) {
	return func() {}
}
