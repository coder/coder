package dormancy

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

const (
	// Time interval between consecutive job runs
	jobInterval = 15 * time.Minute
	// User accounts inactive for `accountDormancyPeriod` will be marked as dormant
	accountDormancyPeriod = 90 * 24 * time.Hour
)

// CheckInactiveUsers function updates status of inactive users from active to dormant
// using default parameters.
func CheckInactiveUsers(ctx context.Context, logger slog.Logger, db database.Store, auditor audit.Auditor) func() {
	return CheckInactiveUsersWithOptions(ctx, logger, db, auditor, jobInterval, accountDormancyPeriod)
}

// CheckInactiveUsersWithOptions function updates status of inactive users from active to dormant
// using provided parameters.
func CheckInactiveUsersWithOptions(ctx context.Context, logger slog.Logger, db database.Store, auditor audit.Auditor, checkInterval, dormancyPeriod time.Duration) func() {
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
			lastSeenAfter := dbtime.Now().Add(-dormancyPeriod)
			logger.Debug(ctx, "check inactive user accounts", slog.F("dormancy_period", dormancyPeriod), slog.F("last_seen_after", lastSeenAfter))

			updatedUsers, err := db.UpdateInactiveUsersToDormant(ctx, database.UpdateInactiveUsersToDormantParams{
				LastSeenAfter: lastSeenAfter,
				UpdatedAt:     dbtime.Now(),
			})
			if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
				logger.Error(ctx, "can't mark inactive users as dormant", slog.Error(err))
				continue
			}

			af := map[string]string{
				"automatic_actor":     "coder",
				"automatic_subsystem": "dormancy",
			}

			wriBytes, err := json.Marshal(af)
			if err != nil {
				logger.Error(ctx, "marshal additional fields", slog.Error(err))
				wriBytes = []byte("{}")
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
					AdditionalFields: wriBytes,
				})
			}
			logger.Debug(ctx, "checking user accounts is done", slog.F("num_dormant_accounts", len(updatedUsers)), slog.F("execution_time", time.Since(startTime)))
		}
	}()

	return func() {
		cancelFunc()
		<-done
	}
}
