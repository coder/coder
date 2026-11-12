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
	"github.com/coder/coder/v2/coderd/rbac"
)

// ErrDeleteForbidden means that the actor can read the automation and cannot
// delete it.
var ErrDeleteForbidden = xerrors.New("not allowed to delete the chat automation")

// ErrQueuedMessages means that a queued message names the automation, so Delete
// refuses. Nothing writes such a message yet. The change that adds the first
// writer also decides how a delete takes the message out of the chat queue,
// and replaces this refusal.
var ErrQueuedMessages = xerrors.New("a queued message names the chat automation")

// LockID returns the advisory lock that every path that runs or changes one
// automation takes first: a run, an edit, a disable, a delete and a rotation.
// The one path without it is the delete_deleted_user_resources trigger, which
// deletes the automations of a user that is soft deleted. The lock lasts to the
// end of the transaction.
func LockID(id uuid.UUID) int64 {
	return database.GenLockID("chat-automation:" + id.String())
}

// Delete deletes an automation in one transaction. It returns the deleted row,
// with its webhook secret, for the audit entry. An automation that does not
// exist, that the actor cannot read, or that belongs to another organization
// gives an error that httpapi.Is404Error reports. An actor that can read it and
// cannot delete it gets ErrDeleteForbidden. An automation that a queued message
// names gives ErrQueuedMessages and nothing changes.
//
// The delete takes the automation lock first, as every other path does, so no
// run adds a queued message between the check and the delete.
//
// actor needs delete on the automation.
func Delete(
	ctx context.Context,
	db database.Store,
	actor rbac.Subject,
	organizationID, id uuid.UUID,
) (database.ChatAutomation, error) {
	actorCtx := dbauthz.As(ctx, actor)

	var deleted database.ChatAutomation
	err := db.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(actorCtx, LockID(id)); err != nil {
			return xerrors.Errorf("lock chat automation: %w", err)
		}
		automation, err := tx.GetChatAutomationByID(actorCtx, id)
		if err != nil {
			return err
		}
		if automation.OrganizationID != organizationID {
			return sql.ErrNoRows
		}
		chatIDs, err := tx.GetChatIDsWithQueuedMessagesByAutomationID(actorCtx, id)
		if dbauthz.IsNotAuthorizedError(err) {
			return ErrDeleteForbidden
		}
		if err != nil {
			return xerrors.Errorf("get chats with queued messages: %w", err)
		}
		if len(chatIDs) > 0 {
			return xerrors.Errorf("%w in %d chats", ErrQueuedMessages, len(chatIDs))
		}
		if err := tx.DeleteChatAutomationByID(actorCtx, id); err != nil {
			return xerrors.Errorf("delete chat automation: %w", err)
		}
		deleted = automation
		return nil
	}, nil)
	if err != nil {
		return database.ChatAutomation{}, err
	}
	return deleted, nil
}
