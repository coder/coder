package budget

import (
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
)

// PeriodWindow is the [Start, End) time window covered by an AI budget
// period. Bounds are in UTC.
type PeriodWindow struct {
	// Start is the inclusive first instant of the window.
	Start time.Time
	// End is the exclusive first instant of the next window.
	End time.Time
}

// CurrentPeriod returns the PeriodWindow containing now (normalized to UTC)
// for the given AI budget period. An unknown budget period returns an error.
func CurrentPeriod(now time.Time, period codersdk.AIBudgetPeriod) (PeriodWindow, error) {
	nowUTC := now.UTC()
	switch period {
	case codersdk.AIBudgetPeriodMonth:
		start := dbtime.StartOfMonth(nowUTC)
		return PeriodWindow{Start: start, End: start.AddDate(0, 1, 0)}, nil
	default:
		return PeriodWindow{}, xerrors.Errorf("unsupported AI budget period: %q", period)
	}
}
