package aibridgedserver

import (
	"context"
	"database/sql"
	"io"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	// unpricedModelsCheckInterval is how often a replica considers reporting.
	// The report itself is sent at most once per
	// unpricedModelsReportFrequency; this only bounds how late a due report
	// can be.
	unpricedModelsCheckInterval = time.Hour

	unpricedModelsReportFrequency      = 7 * 24 * time.Hour
	unpricedModelsReportFrequencyLabel = "week"

	// unpricedModelsLimit caps how many models a single report lists. A
	// deployment can accumulate far more unpriced models than an admin can act
	// on at once, and the remainder is reported as a count.
	unpricedModelsLimit = 100
)

// unpricedModelsReporter periodically reports the models used without a price.
type unpricedModelsReporter struct {
	cancel context.CancelFunc
	closed chan struct{}
}

// NewUnpricedModelsReporter starts a loop that notifies owners about models
// used without a price. Usage of an unpriced model is recorded at a NULL cost,
// so it is neither reported nor enforced against any budget, and an admin
// cannot act on that without being told which models are missing a price.
//
// The set of unpriced models is derived at report time from interceptions and
// the price table rather than tracked as usage happens, so a price set by any
// means, including the price book shipped with an upgrade, removes a model
// from the next report. Nothing is added to the interception path.
func NewUnpricedModelsReporter(ctx context.Context, logger slog.Logger, db database.Store, enqueuer notifications.Enqueuer, clk quartz.Clock) io.Closer {
	closed := make(chan struct{})
	ctx, cancelFunc := context.WithCancel(ctx)
	//nolint:gocritic // The system reports unpriced models without direct user input.
	ctx = dbauthz.AsSystemRestricted(ctx)

	ticker := clk.NewTicker(unpricedModelsCheckInterval)
	doTick := func() {
		if err := reportUnpricedModels(ctx, logger, db, enqueuer, clk); err != nil {
			logger.Error(ctx, "failed to report unpriced AI models", slog.Error(err))
		}
	}

	go func() {
		defer close(closed)
		defer ticker.Stop()
		// Force an initial tick. On a deployment that has never run this
		// report, it only checks in.
		doTick()
		for {
			select {
			case <-ctx.Done():
				logger.Debug(ctx, "closing unpriced AI models reporter")
				return
			case <-ticker.C:
				doTick()
			}
		}
	}()
	return &unpricedModelsReporter{
		cancel: cancelFunc,
		closed: closed,
	}
}

func (r *unpricedModelsReporter) Close() error {
	r.cancel()
	<-r.closed
	return nil
}

// reportUnpricedModels sends one report to every owner covering the models used
// without a price since the last report.
//
// Reporting is guarded twice. The advisory lock keeps concurrent replicas from
// reporting the same window, and the persisted timestamp enforces the
// frequency: the ticker cannot, because it restarts with the process and runs
// on a different phase in each replica.
func reportUnpricedModels(ctx context.Context, logger slog.Logger, db database.Store, enqueuer notifications.Enqueuer, clk quartz.Clock) error {
	now := clk.Now()
	since := now.Add(-unpricedModelsReportFrequency)

	return db.InTx(func(tx database.Store) error {
		acquired, err := tx.TryAcquireLock(ctx, database.LockIDNotifyUnpricedAIModels)
		if err != nil {
			return xerrors.Errorf("acquire unpriced AI models report lock: %w", err)
		}
		if !acquired {
			logger.Debug(ctx, "another replica is reporting unpriced AI models, skipping")
			return nil
		}

		// Firstly, check if this is the first run of the job ever.
		reportLog, err := tx.GetNotificationReportGeneratorLogByTemplate(ctx, notifications.TemplateAIModelsUnpricedReport)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("read report generator log: %w", err)
		}
		if xerrors.Is(err, sql.ErrNoRows) {
			// First run? Check-in the job, and get back after one week. The
			// window before the first check-in belongs to a deployment this
			// report has no history of.
			logger.Info(ctx, "unpriced AI models reporter is executing the job for the first time")
			return upsertUnpricedModelsReportLog(ctx, tx, now)
		}

		// Secondly, check if the job has not been running recently.
		if !reportLog.LastGeneratedAt.IsZero() && reportLog.LastGeneratedAt.Add(unpricedModelsReportFrequency).After(now) {
			return nil // reported recently, no need to report now
		}

		// Thirdly, fetch the models used without a price.
		unpriced, err := tx.GetUnpricedAIModelsSince(ctx, dbtime.Time(since).UTC())
		if err != nil {
			return xerrors.Errorf("fetch unpriced AI models: %w", err)
		}

		if len(unpriced) > 0 {
			owners, err := tx.GetUsers(ctx, database.GetUsersParams{
				RbacRole: []string{codersdk.RoleOwner},
			})
			if err != nil {
				return xerrors.Errorf("fetch owners: %w", err)
			}

			reportData := buildDataForUnpricedModelsReport(unpriced)
			for _, owner := range owners {
				// The enqueuer writes outside this transaction, so a failure
				// here is logged rather than returned: rolling back would
				// discard the timestamp and report the same window again to
				// owners who already received it.
				if _, err := enqueuer.EnqueueWithData(ctx, owner.ID, notifications.TemplateAIModelsUnpricedReport,
					map[string]string{},
					reportData,
					"unpriced_models_reporter",
				); err != nil {
					logger.Warn(ctx, "failed to send a report with unpriced AI models", slog.F("user_id", owner.ID), slog.Error(err))
				}
			}
		}

		// Lastly, record that the window was reported. This happens even when
		// nothing was reported, so the next report covers one week rather than
		// every week since usage was last seen.
		return upsertUnpricedModelsReportLog(ctx, tx, now)
	}, nil)
}

func upsertUnpricedModelsReportLog(ctx context.Context, db database.Store, now time.Time) error {
	if err := db.UpsertNotificationReportGeneratorLog(ctx, database.UpsertNotificationReportGeneratorLogParams{
		NotificationTemplateID: notifications.TemplateAIModelsUnpricedReport,
		LastGeneratedAt:        dbtime.Time(now).UTC(),
	}); err != nil {
		return xerrors.Errorf("update report generator log: %w", err)
	}
	return nil
}

// buildDataForUnpricedModelsReport renders the models most used first, so the
// models dropped by the limit are the ones with the least unreported usage.
// Interception counts order the list but are not reported: they count requests
// rather than spend, and would invite reading them as one.
func buildDataForUnpricedModelsReport(unpriced []database.GetUnpricedAIModelsSinceRow) map[string]any {
	limit := min(len(unpriced), unpricedModelsLimit)
	models := make([]map[string]any, 0, limit)
	for _, row := range unpriced[:limit] {
		models = append(models, map[string]any{
			"provider": row.ProviderType,
			"model":    row.Model,
		})
	}

	data := map[string]any{
		"report_frequency": unpricedModelsReportFrequencyLabel,
		"models":           models,
	}
	if overflow := len(unpriced) - len(models); overflow > 0 {
		data["overflow_count"] = overflow
	}
	return data
}
