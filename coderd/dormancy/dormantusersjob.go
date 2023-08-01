package dormancy

import (
	"context"
	"database/sql"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/database"
)

const (
	checkDuration  = 5 * time.Minute
	dormancyPeriod = 90 * 24 * time.Hour
)

func CheckInactiveUsers(ctx context.Context, logger slog.Logger, db database.Store) func() {
	logger = logger.Named("dormancy")

	ctx, cancelFunc := context.WithCancel(ctx)
	done := make(chan struct{})
	ticker := time.NewTicker(checkDuration)
	go func() {
		defer close(done)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			lastSeenAfter := database.Now().Add(-dormancyPeriod)
			logger.Debug(ctx, "check inactive user accounts", slog.F("dormancy_period", dormancyPeriod), slog.F("last_seen_after", lastSeenAfter))

			updatedUsers, err := db.UpdateInactiveUsersToDormant(ctx, lastSeenAfter)
			if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
				logger.Error(ctx, "can't mark inactive users as dormant", slog.Error(err))
				goto done
			}

			for _, u := range updatedUsers {
				logger.Debug(ctx, "account has been marked as dormant", slog.F("email", u.Email), slog.F("last_seen_at", u.LastSeenAt))
			}
		done:
			logger.Debug(ctx, "checking user accounts is done", slog.F("num_dormant_accounts", len(updatedUsers)))
		}
	}()

	return func() {
		cancelFunc()
		<-done
	}
}
