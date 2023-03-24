package dbpurge

import (
	"context"
	"errors"
	"io"
	"time"

	"golang.org/x/sync/errgroup"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
)

// New creates a new periodically purging database instance.
// It is the caller's responsibility to call Close on the returned instance.
//
// This is for cleaning up old, unused resources from the database that take up space.
func New(ctx context.Context, logger slog.Logger, db database.Store) io.Closer {
	closed := make(chan struct{})
	ctx, cancelFunc := context.WithCancel(ctx)
	go func() {
		defer close(closed)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			var eg errgroup.Group
			eg.Go(func() error {
				return db.DeleteOldWorkspaceAgentStartupLogs(ctx)
			})
			eg.Go(func() error {
				return db.DeleteOldWorkspaceAgentStats(ctx)
			})
			err := eg.Wait()
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				logger.Error(ctx, "failed to purge old database entries", slog.Error(err))
			}
		}
	}()
	return &instance{
		cancel: cancelFunc,
		closed: closed,
	}
}

type instance struct {
	cancel context.CancelFunc
	closed chan struct{}
}

func (i *instance) Close() error {
	i.cancel()
	<-i.closed
	return nil
}
