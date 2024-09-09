package reports

import (
	"context"
	"database/sql"
	"io"
	"time"

	"cdr.dev/slog"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	delay = 5 * time.Minute
)

func NewReportGenerator(ctx context.Context, logger slog.Logger, db database.Store, enqueur notifications.Enqueuer, clk quartz.Clock) io.Closer {
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

			err = reportFailedWorkspaceBuilds(ctx, logger, db, enqueur, clk)
			if err != nil {
				logger.Debug(ctx, "unable to report failed workspace builds")
				return err
			}

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

func reportFailedWorkspaceBuilds(ctx context.Context, logger slog.Logger, db database.Store, _ notifications.Enqueuer, clk quartz.Clock) error {
	const frequencyDays = 7

	templateAdmins, err := db.GetUsers(ctx, database.GetUsersParams{
		RbacRole: []string{codersdk.RoleTemplateAdmin},
	})
	if err != nil {
		return xerrors.Errorf("unable to fetch template admins: %w", err)
	}

	templates, err := db.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{
		Deleted:    false,
		Deprecated: sql.NullBool{Bool: false, Valid: true},
	})
	if err != nil {
		return xerrors.Errorf("unable to fetch active templates: %w", err)
	}

	for _, template := range templates {
		//    1. Fetch failed builds.
		//    2. If failed builds == 0, continue.
		//    3. Fetch template RW users.
		//    4. For user := range template admins + RW users:
		//       1. Check if report is enabled for the person.
		//       2. Check `report_generator_log`.
		//       3. If sent recently, continue
		//       4. Lazy-render the report.
		//       5. Send notification
		//       6. Upsert into `report_generator_log`.
	}

	err = db.DeleteOldReportGeneratorLogs(ctx, frequencyDays)
	if err != nil {
		return xerrors.Errorf("unable to delete old report generator logs: %w", err)
	}
	return nil
}
