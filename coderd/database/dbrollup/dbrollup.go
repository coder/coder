package dbrollup

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

const (
	// DefaultInterval is the default time between rollups.
	// Rollups will be synchronized with the clock so that
	// they happen 13:00, 13:05, 13:10, etc.
	DefaultInterval = 5 * time.Minute
)

type Rolluper struct {
	cancel context.CancelFunc
	closed chan struct{}
	db     database.Store
	logger slog.Logger
}

// New creates a new DB rollup service that periodically runs rollup queries.
// It is the caller's responsibility to call Close on the returned instance.
//
// This is for e.g. generating insights data (template_usage_stats) from
// raw data (workspace_agent_stats, workspace_app_stats).
func New(logger slog.Logger, db database.Store, interval time.Duration) *Rolluper {
	ctx, cancel := context.WithCancel(context.Background())

	r := &Rolluper{
		cancel: cancel,
		closed: make(chan struct{}),
		db:     db,
		logger: logger.Named("dbrollup"),
	}

	//nolint:gocritic // The system rolls up database tables without user input.
	ctx = dbauthz.AsSystemRestricted(ctx)
	go r.start(ctx, interval)

	return r
}

func (r *Rolluper) start(ctx context.Context, interval time.Duration) {
	defer close(r.closed)

	do := func() {
		var eg errgroup.Group

		r.logger.Debug(ctx, "rolling up data")
		now := time.Now()

		// Track whether or not we performed a rollup (we got the advisory lock).
		templateUsageStats := false

		eg.Go(func() error {
			return r.db.InTx(func(tx database.Store) error {
				// Acquire a lock to ensure that only one instance of
				// the rollup is running at a time.
				ok, err := tx.TryAcquireLock(ctx, database.LockIDDBRollup)
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}

				templateUsageStats = true
				return tx.UpsertTemplateUsageStats(ctx)
			}, nil)
		})

		err := eg.Wait()
		if err != nil {
			if database.IsQueryCanceledError(err) {
				return
			}
			// Only log if Close hasn't been called.
			if ctx.Err() == nil {
				r.logger.Error(ctx, "failed to rollup data", slog.Error(err))
			}
		} else {
			r.logger.Debug(ctx,
				"rolled up data",
				slog.F("took", time.Since(now)),
				slog.F("template_usage_stats", templateUsageStats),
			)
		}
	}

	// Perform do immediately and on every tick of the ticker,
	// disregarding the execution time of do. This ensure that
	// the rollup is performed every interval assuming do does
	// not take longer than the interval to execute.
	t := time.NewTicker(time.Microsecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Ensure we're on the interval.
			now := time.Now()
			next := now.Add(interval).Truncate(interval) // Ensure we're on the interval and synced with the clock.
			d := next.Sub(now)
			// Safety check (shouldn't be possible).
			if d <= 0 {
				d = interval
			}
			t.Reset(d)

			do()

			r.logger.Debug(ctx, "next rollup at", slog.F("next", next))
		}
	}
}

func (r *Rolluper) Close() error {
	r.cancel()
	<-r.closed
	return nil
}
