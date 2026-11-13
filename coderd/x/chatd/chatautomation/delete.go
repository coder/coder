// Package chatautomation holds the operations that the API and the agent tool
// share for chat automations, so that each rule exists once.
package chatautomation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
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

// cleanupTimeout bounds DeleteQueuedMessagesByChat.
const cleanupTimeout = 30 * time.Second

// LockByOwner takes the lock of every automation of the owner, in ID order,
// and returns their IDs. The delete of a user calls it before the soft
// delete, whose trigger removes the automations, and passes the IDs to
// DeleteQueuedMessagesByChat after the commit.
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
// each automation, so no run queues a message meanwhile. The chats are locked
// in chat ID order, after the automation locks, as on every other path.
//
// The purge of old chats locks the chats of a batch in the order of its plan,
// so it can form a lock cycle with a transaction that holds one chat and
// waits for another. A cleanup of two or more chats therefore takes the
// advisory lock of the purge job first. A purge tick holds that lock from its
// start to its commit, and it skips when the lock is taken, so the two never
// run side by side.
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
	if len(chatIDs) > 1 {
		if err := tx.AcquireLock(chatdCtx, database.LockIDDBPurge); err != nil {
			return nil, xerrors.Errorf("lock out the purge of old chats: %w", err)
		}
	}

	updates := chatstate.NewPublishBuffer(ps)
	var resumed []database.Chat
	for _, chatID := range chatIDs {
		result, err := removeFromChat(chatdCtx, tx, updates, chatID, automationIDs)
		if err != nil {
			return nil, err
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

// DeleteQueuedMessagesByChat removes the queued messages of the automations
// from their chats, one transaction per chat, and publishes each change after
// its commit. The delete of a user calls it after the soft delete has
// committed, so the automation rows are gone and no run queues a message
// meanwhile. That transaction must not hold a chat row: the purge of old
// chats holds chat rows and then waits for rows that the soft delete removes.
//
// A chat whose removal fails keeps its queued messages. The failure is logged
// with the chat, and the other chats are still cleaned. Automations queue
// messages into chats of their owner, and a chat of a deleted user cannot
// start a turn, so a message that stays never runs.
//
// The soft delete has committed, so a caller that goes away must not stop the
// cleanup: it does not end when ctx is canceled, and it ends after
// cleanupTimeout.
func DeleteQueuedMessagesByChat(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	ps chatstate.Publisher,
	automationIDs []uuid.UUID,
) {
	if len(automationIDs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	//nolint:gocritic // As in DeleteQueuedMessages.
	chatdCtx := dbauthz.AsChatd(ctx)
	chatIDs, err := db.GetChatIDsWithQueuedMessagesByAutomationIDs(chatdCtx, automationIDs)
	if err != nil {
		logger.Warn(ctx, "get chats with queued messages of deleted automations", slog.Error(err))
		return
	}
	for _, chatID := range chatIDs {
		result, err := removeFromChat(chatdCtx, db, ps, chatID, automationIDs)
		if err != nil {
			logger.Warn(ctx, "remove queued messages of deleted automations",
				slog.F("chat_id", chatID), slog.Error(err))
			continue
		}
		if result.Chat == nil {
			continue
		}
		if err := publishStatusChange(ps, *result.Chat); err != nil {
			logger.Warn(ctx, "publish chat status change after automation queue cleanup",
				slog.F("chat_id", chatID), slog.Error(err))
		}
	}
}

// removeFromChat removes the queued messages of the automations from one
// chat. With a store that is not in a transaction, the chat machine opens one
// for this chat and publishes the chat update after its commit. A chat that
// is gone has no queue left, so it gives an empty result.
func removeFromChat(
	chatdCtx context.Context,
	store database.Store,
	ps chatstate.Publisher,
	chatID uuid.UUID,
	automationIDs []uuid.UUID,
) (chatstate.DeleteQueuedMessagesByAutomationResult, error) {
	var result chatstate.DeleteQueuedMessagesByAutomationResult
	err := chatstate.NewChatMachine(store, ps, chatID).Update(chatdCtx, func(chatTx *chatstate.Tx, _ database.Store) error {
		var err error
		result, err = chatTx.DeleteQueuedMessagesByAutomation(chatstate.DeleteQueuedMessagesByAutomationInput{
			AutomationIDs: automationIDs,
		})
		return err
	})
	if errors.Is(err, chatstate.ErrChatNotFound) {
		// The chat went away after the read, and its queue with it.
		return chatstate.DeleteQueuedMessagesByAutomationResult{}, nil
	}
	if err != nil {
		return chatstate.DeleteQueuedMessagesByAutomationResult{}, xerrors.Errorf("remove queued messages from chat %s: %w", chatID, err)
	}
	return result, nil
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
// run queues a message after the cleanup. It then locks the chats, and it
// deletes the automation row last: the purge of old chats locks a chat row
// and then updates the automations that name that chat, so a transaction
// that holds an automation row must not wait for a chat row. See
// DeleteQueuedMessages for the cleanup and its errors.
//
// actor needs delete on the automation. auth checks it before the cleanup,
// which changes chats and can fail with the ID of a chat. The row delete
// checks it again.
func Delete(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	auth rbac.Authorizer,
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
		if err := auth.Authorize(actorCtx, actor, policy.ActionDelete, automation.RBACObject()); err != nil {
			return ErrDeleteForbidden
		}
		publish, err = DeleteQueuedMessages(ctx, logger, tx, ps, []uuid.UUID{id})
		if err != nil {
			return err
		}
		rows, err := tx.DeleteChatAutomationByID(actorCtx, id)
		if err != nil {
			return xerrors.Errorf("delete chat automation: %w", err)
		}
		if rows == 0 {
			// A soft delete of the owner by direct SQL removed the row
			// after the read. That path takes no automation lock.
			return sql.ErrNoRows
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
