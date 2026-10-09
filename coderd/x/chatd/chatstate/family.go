package chatstate

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

// SetFamilyArchivedInput configures [SetFamilyArchived]. The struct
// shape avoids a boolean flag parameter at the API surface; callers
// build it explicitly with named fields for clarity.
type SetFamilyArchivedInput struct {
	// RootID identifies the family root. SetFamilyArchived rejects
	// calls for child chats with [ErrChatNotRoot] and unknown chats
	// with [ErrChatNotFound].
	RootID uuid.UUID
	// Archived is the desired post-call archived value for every
	// family member.
	Archived bool
	// Precondition, when set, runs inside the transaction after the
	// root and every family member row are locked (root first, then
	// children in GetChatFamilyIDsByRootID order) and before any
	// member is updated. A non-nil error aborts the call and rolls
	// back the transaction. Reads issued through tx observe every
	// change committed before the locks were acquired, and writers
	// that lock a family row cannot commit until this call finishes.
	Precondition func(ctx context.Context, tx database.Store) error
}

// SetFamilyArchived runs Update for every chat in the root chat's
// family inside one transaction, applying SetArchived when the chat's
// archived flag differs from the requested value. It owns its
// transaction lifecycle and its [PublishBuffer] lifecycle: pubsub
// publications are buffered while the transaction is open and
// flushed only after a successful commit; the deferred Discard
// suppresses every buffered publication on failure.
//
// On success SetFamilyArchived returns one [database.Chat] per
// family member in the order returned by GetChatFamilyIDsByRootID
// (root first, then children).
//
// Family members that are already in the [StateInvalid] execution
// state cause SetFamilyArchived to return [ErrInvalidState] and roll
// back the cascade even when their archived flag already matches the
// desired value; invalid-state detection is never bypassed.
//
// Family members that are valid and already match the desired
// archived value still run through Update, which increments their
// snapshot version and publishes a fresh snapshot without changing
// the archived flag. Advancing the snapshot version without a field
// change is safe, and it keeps publication behavior uniform while a
// partially archived family converges to the desired state.
func SetFamilyArchived(
	ctx context.Context,
	store database.Store,
	publisher Publisher,
	input SetFamilyArchivedInput,
) ([]database.Chat, error) {
	if store == nil {
		return nil, xerrors.New("chatstate: SetFamilyArchived called with nil store")
	}
	if publisher == nil {
		return nil, xerrors.New("chatstate: SetFamilyArchived called with nil publisher")
	}

	buffer := NewPublishBuffer(publisher)
	defer buffer.Discard()

	var familyChats []database.Chat
	err := store.InTx(func(tx database.Store) error {
		// Lock the root chat first so concurrent archive races on the
		// same family serialize on a stable row.
		root, err := tx.GetChatByIDForUpdate(ctx, input.RootID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChatNotFound
			}
			return xerrors.Errorf("lock root chat for archive: %w", err)
		}
		if root.ParentChatID.Valid {
			return ErrChatNotRoot
		}
		// A family archived by its project delete must stay archived; the
		// root lock serializes this check with that delete.
		if !input.Archived && root.ProjectID.Valid {
			//nolint:gocritic // The caller may no longer read the project.
			inDeletedProject, err := tx.IsChatInDeletedProject(dbauthz.AsChatd(ctx), root.ID)
			if err != nil {
				return xerrors.Errorf("check root chat project: %w", err)
			}
			if inDeletedProject {
				return ErrChatNotFound
			}
		}
		ids, err := tx.GetChatFamilyIDsByRootID(ctx, input.RootID)
		if err != nil {
			return xerrors.Errorf("get chat family: %w", err)
		}
		if len(ids) == 0 {
			return ErrChatNotFound
		}
		if input.Precondition != nil {
			// Lock the remaining members in the same order the update
			// loop below uses, so the precondition sees a stable family.
			for _, id := range ids {
				if id == input.RootID {
					continue
				}
				if _, err := tx.GetChatByIDForUpdate(ctx, id); err != nil {
					return xerrors.Errorf("lock family chat for archive: %w", err)
				}
			}
			if err := input.Precondition(ctx, tx); err != nil {
				return err
			}
		}
		familyChats = make([]database.Chat, 0, len(ids))
		for _, id := range ids {
			var chat database.Chat
			machine := NewChatMachine(tx, buffer, id)
			err := machine.Update(ctx, func(state *Tx, _ database.Store) error {
				// Classify each member so any invalid execution state
				// aborts and rolls back the whole family update, even
				// when that member already has the requested archived
				// value.
				current, from, err := state.loadState()
				if err != nil {
					return err
				}
				if from == StateInvalid {
					return ErrInvalidState
				}
				if current.Archived == input.Archived {
					chat = current
					return nil
				}
				if _, err := state.SetArchived(SetArchivedInput{Archived: input.Archived}); err != nil {
					return err
				}
				chat, err = state.Store().GetChatByID(state.Ctx(), state.ChatID())
				if err != nil {
					return xerrors.Errorf("reload archived chat: %w", err)
				}
				return nil
			})
			if err != nil {
				return err
			}
			familyChats = append(familyChats, chat)
		}
		return nil
	}, nil)
	if err != nil {
		return nil, err
	}
	if err := buffer.Flush(); err != nil {
		return familyChats, err
	}
	return familyChats, nil
}

// ArchiveDeletedChatProject moves every family member of a project that
// tx has already marked deleted from any execution state to [StateXW], or
// [StateXE0] from error, and clears their queues. It must run in the
// project delete transaction; see UpdateChatProjectDeletedByID. It
// publishes a chat:update per chat to publisher, so runners named by the
// old snapshot stop without waiting for heartbeat renewal; pass a
// [PublishBuffer] and flush it after commit. It returns the chats as they
// were before the archive.
func ArchiveDeletedChatProject(ctx context.Context, tx database.Store, publisher Publisher, projectID uuid.UUID) ([]database.Chat, error) {
	if err := tx.LockChatProjectRootChats(ctx, projectID); err != nil {
		return nil, xerrors.Errorf("lock project root chats: %w", err)
	}
	before, err := tx.GetChatProjectChatFamilies(ctx, projectID)
	if err != nil {
		return nil, xerrors.Errorf("get project chats: %w", err)
	}
	if err := tx.ArchiveChatsOfDeletedChatProject(ctx, projectID); err != nil {
		return nil, xerrors.Errorf("archive project chats: %w", err)
	}
	if err := tx.DeleteChatQueuedMessagesOfDeletedChatProject(ctx, projectID); err != nil {
		return nil, xerrors.Errorf("clear project chat queues: %w", err)
	}
	after, err := tx.GetChatProjectChatFamilies(ctx, projectID)
	if err != nil {
		return nil, xerrors.Errorf("reload project chats: %w", err)
	}
	for _, chat := range after {
		if err := publisher.Publish(coderdpubsub.ChatStateUpdateChannel(chat.ID), buildChatUpdateMessage(chat)); err != nil {
			return nil, xerrors.Errorf("publish chat update: %w", err)
		}
	}
	return before, nil
}
