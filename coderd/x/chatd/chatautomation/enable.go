package chatautomation

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/schedule/cron"
)

// ErrUpdateForbidden means that the actor can read the automation and cannot
// update it.
var ErrUpdateForbidden = xerrors.New("not allowed to update the chat automation")

// SetEnabled turns an automation on or off as the actor in ctx and returns
// the row before and after the change. Turning it on schedules the first
// occurrence after now, so occurrences missed while it was off do not run.
// Setting the current state changes nothing. Errors follow Delete, with
// ErrUpdateForbidden for an actor that cannot update the automation.
//
// Like Delete, it takes only the automation lock and no chat row.
func SetEnabled(ctx context.Context, db database.Store, organizationID, id uuid.UUID, enabled bool, now time.Time) (before, after database.ChatAutomation, err error) {
	err = db.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, LockID(id)); err != nil {
			return xerrors.Errorf("lock chat automation: %w", err)
		}
		before, err = tx.GetChatAutomationByID(ctx, id)
		if err != nil {
			return err
		}
		if before.OrganizationID != organizationID {
			return sql.ErrNoRows
		}
		after = before
		if before.Enabled == enabled {
			return nil
		}
		var next sql.NullTime
		if enabled {
			next = NextRunAt(before, now)
		}
		after, err = tx.UpdateChatAutomationEnabled(ctx, database.UpdateChatAutomationEnabledParams{
			ID:                id,
			Enabled:           enabled,
			ScheduleNextRunAt: next,
			UpdatedAt:         now,
		})
		if dbauthz.IsNotAuthorizedError(err) {
			return ErrUpdateForbidden
		}
		if err != nil {
			return xerrors.Errorf("update chat automation: %w", err)
		}
		return nil
	}, nil)
	return before, after, err
}

// NextRunAt returns the first occurrence of the automation's schedule after
// now. It is NULL when the schedule has no later occurrence or this host
// cannot parse it.
func NextRunAt(a database.ChatAutomation, now time.Time) sql.NullTime {
	sched, err := cron.Standard(a.ScheduleCron.String, a.ScheduleTimeZone.String)
	if err != nil {
		return sql.NullTime{}
	}
	next := sched.Next(now)
	return sql.NullTime{Time: next, Valid: !next.IsZero()}
}
