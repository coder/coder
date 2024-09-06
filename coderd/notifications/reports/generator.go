package reports

import (
	"context"
	"io"
	"time"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/quartz"
)

const (
	delay = 5 * time.Minute
)

func NewReportGenerator(ctx context.Context, logger slog.Logger, db database.Store, notificationsEnqueuer notifications.Enqueuer, clk quartz.Clock) io.Closer {
	closed := make(chan struct{})

	ctx, cancelFunc := context.WithCancel(ctx)
	//nolint:gocritic // The system generates periodic reports without direct user input.
	ctx = dbauthz.AsSystemRestricted(ctx)

	// Start the ticker with the initial delay.
	ticker := clk.NewTicker(delay)
	doTick := func(start time.Time) {
		defer ticker.Reset(delay)
		// Start a transaction to grab advisory lock, we don't want to run generator jobs at the same time (multiple replicas).
		if err := db.InTx(func(tx database.Store) error {
			// Acquire a lock to ensure that only one instance of the generator is running at a time.
			ok, err := tx.TryAcquireLock(ctx, database.LockIDReportGenerator)
			if err != nil {
				return err
			}
			if !ok {
				logger.Debug(ctx, "unable to acquire lock for generating periodic reports, skipping")
				return nil
			}

			// TODO:
			//
			// 1. for every user:
			//   1. for every template they administrate:
			//     1. for every enabled report:
			//       1. check last run `report_generator_log`
			//       2. generate report
			//       3. send notification
			//       4. upsert into `report_generator_log`
			// 2. clean stale `report_generator_log` entries

			logger.Info(ctx, "report generator finished", slog.F("duration", clk.Since(start)))

			return nil
		}, nil); err != nil {
			logger.Error(ctx, "failed to generate reports", slog.Error(err))
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
	return &reportGenerator{
		cancel: cancelFunc,
		closed: closed,
	}
}

type reportGenerator struct {
	cancel context.CancelFunc
	closed chan struct{}
}

func (i *reportGenerator) Close() error {
	i.cancel()
	<-i.closed
	return nil
}
