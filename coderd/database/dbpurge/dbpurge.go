package dbpurge

import (
	"context"
	"io"
	"time"

	"golang.org/x/xerrors"

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

	ctx, cancelFunc := context.WithCancel(ctx)
	//nolint:gocritic // The system purges old db records without user input.
	ctx = dbauthz.AsSystemRestricted(ctx)

	// Use time.Nanosecond to force an initial tick. It will be reset to the
	// correct duration after executing once.
	ticker := time.NewTicker(time.Nanosecond)
	doTick := func() {
		defer ticker.Reset(delay)

		start := time.Now()
		// Start a transaction to grab advisory lock, we don't want to run
		// multiple purges at the same time (multiple replicas).
		if err := db.InTx(func(tx database.Store) error {
			// Acquire a lock to ensure that only one instance of the
			// purge is running at a time.
			ok, err := tx.TryAcquireLock(ctx, database.LockIDDBPurge)
			if err != nil {
				return err
			}
			if !ok {
				logger.Debug(ctx, "unable to acquire lock for purging old database entries, skipping")
				return nil
			}

			if err := tx.DeleteOldWorkspaceAgentLogs(ctx); err != nil {
				return xerrors.Errorf("failed to delete old workspace agent logs: %w", err)
			}
			if err := tx.DeleteOldWorkspaceAgentStats(ctx); err != nil {
				return xerrors.Errorf("failed to delete old workspace agent stats: %w", err)
			}
			if err := tx.DeleteOldProvisionerDaemons(ctx); err != nil {
				return xerrors.Errorf("failed to delete old provisioner daemons: %w", err)
			}

			logger.Info(ctx, "purged old database entries", slog.F("duration", time.Since(start)))

			return nil
		}, nil); err != nil {
			logger.Error(ctx, "failed to purge old database entries", slog.Error(err))
			return
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
