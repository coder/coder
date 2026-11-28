// Package chatautomation holds the operations that the API and the agent tool
// share for chat automations, so that each rule exists once.
package chatautomation

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// ErrDeleteForbidden means that the actor can read the automation and cannot
// delete it.
var ErrDeleteForbidden = xerrors.New("not allowed to delete the chat automation")

// LockID returns the advisory lock that every path that runs or changes one
// automation holds to the end of its transaction.
func LockID(id uuid.UUID) int64 {
	return database.GenLockID("chat-automation:" + id.String())
}

// Delete deletes an automation as the actor in ctx and returns the deleted
// row for the audit entry. An automation that does not exist, that the actor
// cannot read, or that belongs to another organization gives an error that
// httpapi.Is404Error reports. An actor that can read it and cannot delete it
// gets ErrDeleteForbidden.
//
// It takes only the automation lock and no chat row, so it cannot form a
// cycle with a run, which locks the chat first. After the commit it passes a
// webhook subscription to unsubscribe, when that is set.
func Delete(ctx context.Context, db database.Store, organizationID, id uuid.UUID, unsubscribe Unsubscriber) (database.ChatAutomation, error) {
	var (
		deleted database.ChatAutomation
		webhook database.ChatAutomationMCPSubscription
		found   bool
	)
	err := db.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, LockID(id)); err != nil {
			return xerrors.Errorf("lock chat automation: %w", err)
		}
		automation, err := tx.GetChatAutomationByID(ctx, id)
		if err != nil {
			return err
		}
		if automation.OrganizationID != organizationID {
			return sql.ErrNoRows
		}
		webhook, found, err = webhookSubscription(ctx, tx, automation)
		if err != nil {
			return err
		}
		err = tx.DeleteChatAutomationByID(ctx, id)
		if dbauthz.IsNotAuthorizedError(err) {
			return ErrDeleteForbidden
		}
		if err != nil {
			return xerrors.Errorf("delete chat automation: %w", err)
		}
		deleted = automation
		return nil
	}, nil)
	if err == nil && found && unsubscribe != nil {
		unsubscribe(ctx, deleted.OwnerID, webhook)
	}
	return deleted, err
}
