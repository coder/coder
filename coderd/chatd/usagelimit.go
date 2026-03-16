package chatd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

// ComputeUsagePeriodBounds returns the UTC-aligned start and end bounds for the
// active usage-limit period containing now.
func ComputeUsagePeriodBounds(now time.Time, period codersdk.ChatUsageLimitPeriod) (start, end time.Time) {
	utcNow := now.UTC()

	switch period {
	case codersdk.ChatUsageLimitPeriodDay:
		start = time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 0, 1)
	case codersdk.ChatUsageLimitPeriodWeek:
		// Walk backward to Monday of the current ISO week.
		// ISO 8601 weeks always start on Monday, so this never
		// crosses an ISO-week boundary.
		start = time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), 0, 0, 0, 0, time.UTC)
		for start.Weekday() != time.Monday {
			start = start.AddDate(0, 0, -1)
		}
		end = start.AddDate(0, 0, 7)
	case codersdk.ChatUsageLimitPeriodMonth:
		start = time.Date(utcNow.Year(), utcNow.Month(), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 1, 0)
	default:
		panic(fmt.Sprintf("unknown chat usage limit period: %q", period))
	}

	return start, end
}

// ResolveUsageLimitStatus resolves the current usage-limit status for userID.
//
// Note: There is a potential race condition where two concurrent messages
// from the same user can both pass the limit check if processed in
// parallel, allowing brief overage. This is acceptable because:
//   - Cost is only known after the LLM API returns.
//   - Overage is bounded by message cost × concurrency.
//   - Fail-open is the deliberate design choice for this feature.
//
// Architecture note: today this path enforces one period globally
// (day/week/month) from config.
// To support simultaneous periods, add nullable
// daily/weekly/monthly_limit_micros columns on override tables, where NULL
// means no limit for that period.
// Then scan spend once over the widest active window with conditional SUMs
// for each period and compare each spend/limit pair Go-side, blocking on
// whichever period is tightest.
func ResolveUsageLimitStatus(ctx context.Context, db database.Store, userID uuid.UUID, now time.Time) (*codersdk.ChatUsageLimitStatus, error) {
	//nolint:gocritic // AsChatd provides narrowly-scoped daemon access for
	// deployment config reads and cross-user chat spend aggregation.
	authCtx := dbauthz.AsChatd(ctx)

	config, err := db.GetChatUsageLimitConfig(authCtx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil //nolint:nilnil // Nil status cleanly signals disabled limits.
		}
		return nil, err
	}
	if !config.Enabled {
		return nil, nil //nolint:nilnil // Nil status cleanly signals disabled limits.
	}

	period, ok := mapDBPeriodToSDK(config.Period)
	if !ok {
		return nil, xerrors.Errorf("invalid chat usage limit period %q", config.Period)
	}

	// Resolve effective limit in a single query:
	// individual override > group limit > global default.
	effectiveLimit, err := db.ResolveUserChatSpendLimit(authCtx, userID)
	if err != nil {
		return nil, err
	}
	// -1 means limits are disabled (shouldn't happen since we checked above,
	// but handle gracefully).
	if effectiveLimit < 0 {
		return nil, nil //nolint:nilnil // Nil status cleanly signals disabled limits.
	}

	start, end := ComputeUsagePeriodBounds(now, period)

	spendTotal, err := db.GetUserChatSpendInPeriod(authCtx, database.GetUserChatSpendInPeriodParams{
		UserID:    userID,
		StartTime: start,
		EndTime:   end,
	})
	if err != nil {
		return nil, err
	}

	return &codersdk.ChatUsageLimitStatus{
		IsLimited:        true,
		Period:           period,
		SpendLimitMicros: &effectiveLimit,
		CurrentSpend:     spendTotal,
		PeriodStart:      start,
		PeriodEnd:        end,
	}, nil
}

func mapDBPeriodToSDK(dbPeriod string) (codersdk.ChatUsageLimitPeriod, bool) {
	switch dbPeriod {
	case string(codersdk.ChatUsageLimitPeriodDay):
		return codersdk.ChatUsageLimitPeriodDay, true
	case string(codersdk.ChatUsageLimitPeriodWeek):
		return codersdk.ChatUsageLimitPeriodWeek, true
	case string(codersdk.ChatUsageLimitPeriodMonth):
		return codersdk.ChatUsageLimitPeriodMonth, true
	default:
		return "", false
	}
}
