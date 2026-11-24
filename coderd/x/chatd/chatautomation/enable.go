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
	"github.com/coder/coder/v2/codersdk"
)

// ErrUpdateForbidden means that the actor can read the automation and cannot
// update it.
var ErrUpdateForbidden = xerrors.New("not allowed to update the chat automation")

// Update turns an automation on or off as the actor in ctx and returns the
// row before and after the change. Turning it on schedules the first
// occurrence after now, so occurrences missed while it was off do not run.
// Setting the current state changes nothing, but still needs update
// permission. Errors follow Delete, with ErrUpdateForbidden for an actor that
// cannot update the automation.
//
// Like Delete, it takes only the automation lock and no chat row. Turning a
// webhook subscription off unsubscribes it after the commit, and turning it
// on subscribes it again at the next tick of the automation loop.
func Update(ctx context.Context, db database.Store, organizationID, id uuid.UUID, req codersdk.UpdateChatAutomationRequest, now time.Time, unsubscribe Unsubscriber) (before, after database.ChatAutomation, err error) {
	var (
		webhook database.ChatAutomationMCPSubscription
		found   bool
	)
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
		var next sql.NullTime
		if req.Enabled {
			next = NextRunAt(before, now)
		}
		after, err = tx.UpdateChatAutomationEnabled(ctx, database.UpdateChatAutomationEnabledParams{
			ID:                id,
			Enabled:           req.Enabled,
			ScheduleNextRunAt: next,
			UpdatedAt:         now,
		})
		if dbauthz.IsNotAuthorizedError(err) {
			return ErrUpdateForbidden
		}
		if err != nil {
			return xerrors.Errorf("update chat automation: %w", err)
		}
		if before.Enabled == req.Enabled {
			return nil
		}
		webhook, found, err = webhookSubscription(ctx, tx, after)
		if err != nil || !found || !req.Enabled {
			return err
		}
		return tx.SetChatAutomationWebhookRenewAt(ctx, database.SetChatAutomationWebhookRenewAtParams{AutomationID: id, RenewAt: now, Now: now})
	}, nil)
	if err == nil && found && !req.Enabled && unsubscribe != nil {
		unsubscribe(ctx, after.OwnerID, webhook)
	}
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
