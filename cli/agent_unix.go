//go:build !windows

package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"cdr.dev/slog"
)

func agentStartPPROFOnUSR1(ctx context.Context, logger slog.Logger, pprofAddress string) (srvClose func()) {
	ctx, cancel := context.WithCancel(ctx)

	usr1 := make(chan os.Signal, 1)
	signal.Notify(usr1, syscall.SIGUSR1)
	go func() {
		defer close(usr1)
		defer signal.Stop(usr1)

		select {
		case <-usr1:
			signal.Stop(usr1)
			srvClose := serveHandler(ctx, logger, nil, pprofAddress, "pprof")
			defer srvClose()
		case <-ctx.Done():
			return
		}
		<-ctx.Done() // Prevent defer close until done.
	}()

	return func() {
		cancel()
		<-usr1 // Wait until usr1 is closed, ensures srvClose was run.
	}
}
