package chatd

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

// ComputePeriodBounds returns the UTC-aligned start and end bounds for the
// active usage-limit period containing now.
func ComputePeriodBounds(now time.Time, period codersdk.ChatUsageLimitPeriod) (start, end time.Time) {
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
	}

	return start, end
}

// ResolveUsageLimitStatus resolves the current usage-limit status for userID.
func ResolveUsageLimitStatus(ctx context.Context, db database.Store, userID uuid.UUID, now time.Time) (*codersdk.ChatUsageLimitStatus, error) {
	//nolint:gocritic // Shared HTTP and daemon usage-limit checks need
	// deployment-config access plus cross-user spend reads.
	authCtx := dbauthz.AsSystemRestricted(ctx)

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

	start, end := ComputePeriodBounds(now, period)

	effectiveLimit := config.DefaultLimitMicros
	override, err := db.GetChatUsageLimitOverrideByUserID(authCtx, userID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	} else {
		effectiveLimit = override.LimitMicros
	}

	spendTotal, err := db.GetUserChatSpendInPeriod(authCtx, database.GetUserChatSpendInPeriodParams{
		UserID:    userID,
		StartTime: start,
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
