package dormancy

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/quartz"
)

const (
	// Time interval between consecutive job runs
	jobInterval = 15 * time.Minute
	// User accounts inactive for `accountDormancyPeriod` will be marked as dormant
	accountDormancyPeriod = 90 * 24 * time.Hour
)

// CheckInactiveUsers function updates status of inactive users from active to dormant
// using default parameters.
func CheckInactiveUsers(ctx context.Context, logger slog.Logger, clk quartz.Clock, db database.Store, auditor audit.Auditor) func() {
	return CheckInactiveUsersWithOptions(ctx, logger, clk, db, auditor, jobInterval, accountDormancyPeriod)
}

// CheckInactiveUsersWithOptions function updates status of inactive users from active to dormant
// using provided parameters.
func CheckInactiveUsersWithOptions(ctx context.Context, logger slog.Logger, clk quartz.Clock, db database.Store, auditor audit.Auditor, checkInterval, dormancyPeriod time.Duration) func() {
	logger = logger.Named("dormancy")

	ctx, cancelFunc := context.WithCancel(ctx)
	tf := clk.TickerFunc(ctx, checkInterval, func() error {
		startTime := time.Now()
		lastSeenAfter := dbtime.Now().Add(-dormancyPeriod)
		logger.Debug(ctx, "check inactive user accounts", slog.F("dormancy_period", dormancyPeriod), slog.F("last_seen_after", lastSeenAfter))

		updatedUsers, err := db.UpdateInactiveUsersToDormant(ctx, database.UpdateInactiveUsersToDormantParams{
			LastSeenAfter: lastSeenAfter,
			UpdatedAt:     dbtime.Now(),
		})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			logger.Error(ctx, "can't mark inactive users as dormant", slog.Error(err))
			return nil
		}

		for _, u := range updatedUsers {
			logger.Info(ctx, "account has been marked as dormant", slog.F("email", u.Email), slog.F("last_seen_at", u.LastSeenAt))
			audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.User]{
				Audit:            auditor,
				Log:              logger,
				UserID:           u.ID,
				Action:           database.AuditActionWrite,
				Old:              database.User{ID: u.ID, Username: u.Username, Status: database.UserStatusActive},
				New:              database.User{ID: u.ID, Username: u.Username, Status: database.UserStatusDormant},
				Status:           http.StatusOK,
				AdditionalFields: audit.BackgroundTaskFieldsBytes(ctx, logger, audit.BackgroundSubsystemDormancy),
			})
		}
		logger.Debug(ctx, "checking user accounts is done", slog.F("num_dormant_accounts", len(updatedUsers)), slog.F("execution_time", time.Since(startTime)))
		return nil
	})

	return func() {
		cancelFunc()
		_ = tf.Wait()
	}
}
