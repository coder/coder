package dbpurge
import (
	"fmt"
	"errors"
	"context"
	"io"
	"time"
	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/quartz"
)
const (
	delay          = 10 * time.Minute
	maxAgentLogAge = 7 * 24 * time.Hour
)
// New creates a new periodically purging database instance.
// It is the caller's responsibility to call Close on the returned instance.
//
// This is for cleaning up old, unused resources from the database that take up space.
func New(ctx context.Context, logger slog.Logger, db database.Store, clk quartz.Clock) io.Closer {
	closed := make(chan struct{})
	ctx, cancelFunc := context.WithCancel(ctx)
	//nolint:gocritic // The system purges old db records without user input.
	ctx = dbauthz.AsSystemRestricted(ctx)
	// Start the ticker with the initial delay.
	ticker := clk.NewTicker(delay)
	doTick := func(start time.Time) {
		defer ticker.Reset(delay)
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
			deleteOldWorkspaceAgentLogsBefore := start.Add(-maxAgentLogAge)
			if err := tx.DeleteOldWorkspaceAgentLogs(ctx, deleteOldWorkspaceAgentLogsBefore); err != nil {
				return fmt.Errorf("failed to delete old workspace agent logs: %w", err)
			}
			if err := tx.DeleteOldWorkspaceAgentStats(ctx); err != nil {
				return fmt.Errorf("failed to delete old workspace agent stats: %w", err)
			}
			if err := tx.DeleteOldProvisionerDaemons(ctx); err != nil {
				return fmt.Errorf("failed to delete old provisioner daemons: %w", err)
			}
			if err := tx.DeleteOldNotificationMessages(ctx); err != nil {
				return fmt.Errorf("failed to delete old notification messages: %w", err)
			}
			logger.Debug(ctx, "purged old database entries", slog.F("duration", clk.Since(start)))
			return nil
		}, database.DefaultTXOptions().WithID("db_purge")); err != nil {
			logger.Error(ctx, "failed to purge old database entries", slog.Error(err))
			return
		}
	}
	go func() {
		defer close(closed)
		defer ticker.Stop()
		// Force an initial tick.
		doTick(dbtime.Time(clk.Now()).UTC())
		for {
			select {
			case <-ctx.Done():
				return
			case tick := <-ticker.C:
				ticker.Stop()
				doTick(dbtime.Time(tick).UTC())
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
