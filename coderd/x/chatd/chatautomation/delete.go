// Package chatautomation holds the operations that the API and the agent tool
// share for chat automations, so that each rule exists once.
package chatautomation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

// ErrDeleteForbidden means that the actor can read the automation and cannot
// delete it.
var ErrDeleteForbidden = xerrors.New("not allowed to delete the chat automation")

// LockID returns the advisory lock that every path that runs or changes one
// automation takes first: a run, an edit, a disable, a delete, a rotation and
// the delete of its owner. The one path without it is a soft delete of the
// owner by direct SQL, where only the delete_deleted_user_resources trigger
// runs. The lock lasts to the end of the transaction.
func LockID(id uuid.UUID) int64 {
	return database.GenLockID("chat-automation:" + id.String())
}

// LockByOwner takes the lock of every automation of the owner, in ID order,
// and returns their IDs. The delete of a user calls it before the soft
// delete, whose trigger removes the automations, and then passes the IDs to
// DeleteQueuedMessages.
func LockByOwner(ctx context.Context, tx database.Store, ownerID uuid.UUID) ([]uuid.UUID, error) {
	//nolint:gocritic // The actor deletes a user and need not be able to read that user's automations.
	ids, err := tx.GetChatAutomationIDsByOwnerID(dbauthz.AsChatd(ctx), ownerID)
	if err != nil {
		return nil, xerrors.Errorf("get chat automations of owner: %w", err)
	}
	for _, id := range ids {
		if err := tx.AcquireLock(ctx, LockID(id)); err != nil {
			return nil, xerrors.Errorf("lock chat automation: %w", err)
		}
	}
	return ids, nil
}

// DeleteQueuedMessages removes the queued messages of the automations from
// their chats, in the transaction of the caller. The caller holds the lock of
// each automation, so no run queues a message meanwhile. The automation rows
// can be gone already. The chats are locked in chat ID order, after the
// automation locks, as on every other path.
//
// A chat in an invalid state refuses the removal, as it refuses every
// transition but the reconcile: the error wraps chatstate.ErrInvalidState and
// names the chat.
//
// The returned function publishes the chat changes. The caller runs it after
// the transaction has committed.
func DeleteQueuedMessages(
	ctx context.Context,
	logger slog.Logger,
	tx database.Store,
	ps chatstate.Publisher,
	automationIDs []uuid.UUID,
) (publish func(), err error) {
	// The removal follows from the change to the automation. It must not
	// depend on what the actor can do to a chat, so it runs as chatd.
	//nolint:gocritic // See above.
	chatdCtx := dbauthz.AsChatd(ctx)
	chatIDs, err := tx.GetChatIDsWithQueuedMessagesByAutomationIDs(chatdCtx, automationIDs)
	if err != nil {
		return nil, xerrors.Errorf("get chats with queued messages: %w", err)
	}

	updates := chatstate.NewPublishBuffer(ps)
	var resumed []database.Chat
	for _, chatID := range chatIDs {
		var result chatstate.DeleteQueuedMessagesByAutomationResult
		err := chatstate.NewChatMachine(tx, updates, chatID).Update(chatdCtx, func(chatTx *chatstate.Tx, _ database.Store) error {
			var err error
			result, err = chatTx.DeleteQueuedMessagesByAutomation(chatstate.DeleteQueuedMessagesByAutomationInput{
				AutomationIDs: automationIDs,
			})
			return err
		})
		if errors.Is(err, chatstate.ErrChatNotFound) {
			// The chat went away after the read, and its queue with it.
			continue
		}
		if err != nil {
			return nil, xerrors.Errorf("remove queued messages from chat %s: %w", chatID, err)
		}
		if result.Chat != nil {
			resumed = append(resumed, *result.Chat)
		}
	}

	return func() {
		if err := updates.Flush(); err != nil {
			logger.Warn(ctx, "publish chat updates after automation queue cleanup", slog.Error(err))
		}
		// A chat that left the paused state changes in the chat list too,
		// as it does when a person deletes the queued message.
		for _, chat := range resumed {
			if err := publishStatusChange(ps, chat); err != nil {
				logger.Warn(ctx, "publish chat status change after automation queue cleanup",
					slog.F("chat_id", chat.ID), slog.Error(err))
			}
		}
	}, nil
}

func publishStatusChange(ps chatstate.Publisher, chat database.Chat) error {
	payload, err := json.Marshal(codersdk.ChatWatchEvent{
		Kind: codersdk.ChatWatchEventKindStatusChange,
		Chat: db2sdk.ChatWatchEventChat(chat, nil),
	})
	if err != nil {
		return xerrors.Errorf("marshal chat watch event: %w", err)
	}
	return ps.Publish(coderdpubsub.ChatWatchEventChannel(chat.OwnerID), payload)
}

// Delete deletes an automation and its queued messages in one transaction. It
// returns the deleted row, with its webhook secret, for the audit entry. An
// automation that does not exist, that the actor cannot read, or that belongs
// to another organization gives an error that httpapi.Is404Error reports. An
// actor that can read it and cannot delete it gets ErrDeleteForbidden.
//
// The delete takes the automation lock first, as every other path does, so no
// run queues a message after the cleanup. See DeleteQueuedMessages for the
// cleanup and its errors.
//
// actor needs delete on the automation.
func Delete(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	ps chatstate.Publisher,
	actor rbac.Subject,
	organizationID, id uuid.UUID,
) (database.ChatAutomation, error) {
	actorCtx := dbauthz.As(ctx, actor)

	var deleted database.ChatAutomation
	var publish func()
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
		rows, err := tx.DeleteChatAutomationByID(actorCtx, id)
		if dbauthz.IsNotAuthorizedError(err) {
			return ErrDeleteForbidden
		}
		if err != nil {
			return xerrors.Errorf("delete chat automation: %w", err)
		}
		if rows == 0 {
			// A soft delete of the owner by direct SQL removed the row
			// after the read. That path takes no automation lock.
			return sql.ErrNoRows
		}
		publish, err = DeleteQueuedMessages(ctx, logger, tx, ps, []uuid.UUID{id})
		if err != nil {
			return err
		}
		deleted = automation
		return nil
	}, nil)
	if err != nil {
		return database.ChatAutomation{}, err
	}
	publish()
	return deleted, nil
}
