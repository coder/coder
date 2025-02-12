package dbrollup

import (
	"context"
	"flag"
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

type Event struct {
	Init               bool `json:"-"`
	TemplateUsageStats bool `json:"template_usage_stats"`
}

type Rolluper struct {
	cancel   context.CancelFunc
	closed   chan struct{}
	db       database.Store
	logger   slog.Logger
	interval time.Duration
	event    chan<- Event
}

type Option func(*Rolluper)

// WithInterval sets the interval between rollups.
func WithInterval(interval time.Duration) Option {
	return func(r *Rolluper) {
		r.interval = interval
	}
}

// WithEventChannel sets the event channel to use for rollup events.
//
// This is only used for testing.
func WithEventChannel(ch chan<- Event) Option {
	if flag.Lookup("test.v") == nil {
		panic("developer error: WithEventChannel is not to be used outside of tests")
	}
	return func(r *Rolluper) {
		r.event = ch
	}
}

// New creates a new DB rollup service that periodically runs rollup queries.
// It is the caller's responsibility to call Close on the returned instance.
//
// This is for e.g. generating insights data (template_usage_stats) from
// raw data (workspace_agent_stats, workspace_app_stats).
func New(logger slog.Logger, db database.Store, opts ...Option) *Rolluper {
	ctx, cancel := context.WithCancel(context.Background())

	r := &Rolluper{
		cancel:   cancel,
		closed:   make(chan struct{}),
		db:       db,
		logger:   logger,
		interval: DefaultInterval,
	}

	for _, opt := range opts {
		opt(r)
	}

	//nolint:gocritic // The system rolls up database tables without user input.
	ctx = dbauthz.AsSystemRestricted(ctx)
	go r.start(ctx)

	return r
}

func (r *Rolluper) start(ctx context.Context) {
	defer close(r.closed)

	do := func() {
		var eg errgroup.Group

		r.logger.Debug(ctx, "rolling up data")
		now := time.Now()

		// Track whether or not we performed a rollup (we got the advisory lock).
		var ev Event

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

				ev.TemplateUsageStats = true
				return tx.UpsertTemplateUsageStats(ctx)
			}, database.DefaultTXOptions().WithID("db_rollup"))
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
			return
		}

		r.logger.Debug(ctx,
			"rolled up data",
			slog.F("took", time.Since(now)),
			slog.F("event", ev),
		)

		// For testing.
		if r.event != nil {
			select {
			case <-ctx.Done():
				return
			case r.event <- ev:
			}
		}
	}

	// For testing.
	if r.event != nil {
		select {
		case <-ctx.Done():
			return
		case r.event <- Event{Init: true}:
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
			next := now.Add(r.interval).Truncate(r.interval) // Ensure we're on the interval and synced with the clock.
			d := next.Sub(now)
			// Safety check (shouldn't be possible).
			if d <= 0 {
				d = r.interval
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
