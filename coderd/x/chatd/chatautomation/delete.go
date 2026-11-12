// Package chatautomation holds the operations that the API and the agent tool
// share for chat automations, so that each rule exists once.
package chatautomation

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
)

// LockID returns the advisory lock that every path that runs or changes one
// automation takes first: a run, an edit, a disable, a delete and a rotation.
// The lock lasts to the end of the transaction.
func LockID(id uuid.UUID) int64 {
	return database.GenLockID("chat-automation:" + id.String())
}

// Delete deletes an automation and the messages that it queued, in one
// transaction. It returns the deleted row, with its webhook secret, for the
// audit entry. An automation that does not exist, that actor cannot read, or
// that belongs to another organization gives an error that
// httpapi.Is404Error reports.
//
// The lock order is the one of every other path: the automation lock first,
// then the lock of each chat that holds a queued message of the automation,
// in chat ID order, taken like any chat transition does (the row lock and an
// increment of snapshot_version). Only then the queued messages and the
// automation go. The chats publish their change after the commit.
//
// actor needs delete on the automation. The chat locks run as chatd, because
// a role that grants delete on automations, such as a custom role, need not
// grant update on the owner's chats, and the delete of the automation is what
// authorizes the change of its queue.
func Delete(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	publisher chatstate.Publisher,
	actor rbac.Subject,
	organizationID, id uuid.UUID,
) (database.ChatAutomation, error) {
	actorCtx := dbauthz.As(ctx, actor)
	//nolint:gocritic // Chat locks for a delete that the actor is authorized to make.
	chatdCtx := dbauthz.AsChatd(ctx)
	buffer := chatstate.NewPublishBuffer(publisher)
	defer buffer.Discard()

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
		// Every run takes the automation lock first, so no run adds a
		// queued message to a chat after this read.
		chatIDs, err := tx.GetChatIDsWithQueuedMessagesByAutomationID(actorCtx, id)
		if err != nil {
			return xerrors.Errorf("get chats with queued messages: %w", err)
		}
		for _, chatID := range chatIDs {
			machine := chatstate.NewChatMachine(tx, buffer, chatID)
			err := machine.Update(chatdCtx, func(_ *chatstate.Tx, store database.Store) error {
				return store.DeleteChatQueuedMessagesByAutomationID(actorCtx, database.DeleteChatQueuedMessagesByAutomationIDParams{
					AutomationID: id,
					ChatID:       chatID,
				})
			})
			// The chat is gone, and its queued messages with it.
			if err != nil && !errors.Is(err, chatstate.ErrChatNotFound) {
				return xerrors.Errorf("delete queued messages of chat %s: %w", chatID, err)
			}
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
	if err := buffer.Flush(); err != nil {
		logger.Warn(ctx, "publish the queue change of a deleted chat automation",
			slog.F("automation_id", id), slog.Error(err))
	}
	return deleted, nil
}
