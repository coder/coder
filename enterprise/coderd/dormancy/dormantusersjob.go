package dormancy

import (
	"context"
	"database/sql"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
)

const (
	// Time interval between consecutive job runs
	jobInterval = 15 * time.Minute
	// User accounts inactive for `accountDormancyPeriod` will be marked as dormant
	accountDormancyPeriod = 90 * 24 * time.Hour
)

// CheckInactiveUsers function updates status of inactive users from active to dormant
// using default parameters.
func CheckInactiveUsers(ctx context.Context, logger slog.Logger, db database.Store) func() {
	return CheckInactiveUsersWithOptions(ctx, logger, db, jobInterval, accountDormancyPeriod)
}

// CheckInactiveUsersWithOptions function updates status of inactive users from active to dormant
// using provided parameters.
func CheckInactiveUsersWithOptions(ctx context.Context, logger slog.Logger, db database.Store, checkInterval, dormancyPeriod time.Duration) func() {
	logger = logger.Named("dormancy")

	ctx, cancelFunc := context.WithCancel(ctx)
	done := make(chan struct{})
	ticker := time.NewTicker(checkInterval)
	go func() {
		defer close(done)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			startTime := time.Now()
			lastSeenAfter := database.Now().Add(-dormancyPeriod)
			logger.Debug(ctx, "check inactive user accounts", slog.F("dormancy_period", dormancyPeriod), slog.F("last_seen_after", lastSeenAfter))

			updatedUsers, err := db.UpdateInactiveUsersToDormant(ctx, database.UpdateInactiveUsersToDormantParams{
				LastSeenAfter: lastSeenAfter,
				UpdatedAt:     database.Now(),
			})
			if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
				logger.Error(ctx, "can't mark inactive users as dormant", slog.Error(err))
				continue
			}

			for _, u := range updatedUsers {
				logger.Info(ctx, "account has been marked as dormant", slog.F("email", u.Email), slog.F("last_seen_at", u.LastSeenAt))
			}
			logger.Debug(ctx, "checking user accounts is done", slog.F("num_dormant_accounts", len(updatedUsers)), slog.F("execution_time", time.Since(startTime)))
		}
	}()

	return func() {
		cancelFunc()
		<-done
	}
}
