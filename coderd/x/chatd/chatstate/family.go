package chatstate

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// SetFamilyArchivedInput configures [SetFamilyArchived]. The struct
// shape avoids a boolean flag parameter at the API surface; callers
// build it explicitly with named fields for clarity.
type SetFamilyArchivedInput struct {
	// RootID identifies the chat at the top of the cascade. It must be a
	// root or chat kind row: subagents are rejected with [ErrChatNotRoot]
	// and unknown chats with [ErrChatNotFound]. Archiving a tree root is
	// rejected with [ErrChatTreeRootArchive].
	RootID uuid.UUID
	// Archived is the desired post-call archived value. Archiving covers
	// the whole subtree (named descendants and subagents). Unarchiving
	// covers the chat and its direct subagents only and is rejected with
	// [ErrChatParentArchived] while the chat's parent is archived.
	Archived bool
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
// affected chat, parents before children.
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
		if root.Kind == database.ChatKindSubagent {
			return ErrChatNotRoot
		}
		if root.Kind == database.ChatKindRoot && input.Archived {
			return ErrChatTreeRootArchive
		}
		var ids []uuid.UUID
		if input.Archived {
			ids, err = tx.GetChatSubtreeIDs(ctx, input.RootID)
		} else {
			// The parent row is read, not locked. This transaction only
			// locks the chat and rows below it.
			if root.ParentChatID.Valid {
				parent, err := tx.GetChatByID(ctx, root.ParentChatID.UUID)
				if err != nil {
					return xerrors.Errorf("get parent chat: %w", err)
				}
				if parent.Archived {
					return ErrChatParentArchived
				}
			}
			ids, err = tx.GetChatAndSubagentIDs(ctx, input.RootID)
		}
		if err != nil {
			return xerrors.Errorf("get chat family: %w", err)
		}
		if len(ids) == 0 {
			return ErrChatNotFound
		}
		familyChats = make([]database.Chat, 0, len(ids))
		seen := make(map[uuid.UUID]struct{}, len(ids))
		// Each member is locked by machine.Update. A child created under
		// a member between the subtree read and that member's lock is
		// committed before the lock is granted, so the subtree is
		// re-read after every pass until no unlocked member remains.
		for len(ids) > 0 {
			for _, id := range ids {
				if _, done := seen[id]; done {
					continue
				}
				seen[id] = struct{}{}
				chat, err := setMemberArchived(ctx, tx, buffer, id, input.Archived)
				if err != nil {
					return err
				}
				familyChats = append(familyChats, chat)
			}
			if !input.Archived {
				break
			}
			latest, err := tx.GetChatSubtreeIDs(ctx, input.RootID)
			if err != nil {
				return xerrors.Errorf("re-read chat subtree: %w", err)
			}
			ids = ids[:0]
			for _, id := range latest {
				if _, done := seen[id]; !done {
					ids = append(ids, id)
				}
			}
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

// setMemberArchived applies SetArchived(archived) to one chat through its
// machine, locking the row. A member whose archived flag already matches
// is still classified so an invalid execution state aborts the cascade.
//
//nolint:revive // Existing API takes the target archive state as a boolean.
func setMemberArchived(
	ctx context.Context,
	tx database.Store,
	buffer *PublishBuffer,
	id uuid.UUID,
	archived bool,
) (database.Chat, error) {
	var chat database.Chat
	machine := NewChatMachine(tx, buffer, id)
	err := machine.Update(ctx, func(state *Tx, _ database.Store) error {
		current, from, err := state.loadState()
		if err != nil {
			return err
		}
		if from == StateInvalid {
			return ErrInvalidState
		}
		if current.Archived == archived {
			chat = current
			return nil
		}
		if _, err := state.SetArchived(SetArchivedInput{Archived: archived}); err != nil {
			return err
		}
		chat, err = state.Store().GetChatByID(state.Ctx(), state.ChatID())
		if err != nil {
			return xerrors.Errorf("reload archived chat: %w", err)
		}
		return nil
	})
	if err != nil {
		return database.Chat{}, err
	}
	return chat, nil
}
