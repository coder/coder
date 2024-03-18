package dbpurge

import (
	"context"
	"errors"
	"io"
	"time"

	"golang.org/x/sync/errgroup"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

const (
	delay = 10 * time.Minute
)

// New creates a new periodically purging database instance.
// It is the caller's responsibility to call Close on the returned instance.
//
// This is for cleaning up old, unused resources from the database that take up space.
func New(ctx context.Context, logger slog.Logger, db database.Store) io.Closer {
	closed := make(chan struct{})
	logger = logger.Named("dbpurge")

	ctx, cancelFunc := context.WithCancel(ctx)
	//nolint:gocritic // The system purges old db records without user input.
	ctx = dbauthz.AsSystemRestricted(ctx)

	// Use time.Nanosecond to force an initial tick. It will be reset to the
	// correct duration after executing once.
	ticker := time.NewTicker(time.Nanosecond)
	doTick := func() {
		defer ticker.Reset(delay)

		var eg errgroup.Group
		eg.Go(func() error {
			return db.DeleteOldWorkspaceAgentLogs(ctx)
		})
		eg.Go(func() error {
			return db.DeleteOldWorkspaceAgentStats(ctx)
		})
		eg.Go(func() error {
			return db.DeleteOldProvisionerDaemons(ctx)
		})
		err := eg.Wait()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			logger.Error(ctx, "failed to purge old database entries", slog.Error(err))
		}
	}

	go func() {
		defer close(closed)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ticker.Stop()
				doTick()
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
