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
	// with [ErrChatNotFound]. Unarchiving a root whose project is
	// deleted also returns [ErrChatNotFound].
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
		// A family archived by its project delete must stay archived. The
		// cleanup skips archived roots, so hold the project FOR SHARE: a
		// delete then waits for this unarchive, and its cleanup sees the
		// family unarchived.
		if !input.Archived && root.ProjectID.Valid {
			//nolint:gocritic // The caller authorized the root chat, not its project.
			_, err := tx.GetChatProjectByIDForShare(dbauthz.AsChatd(ctx), root.ProjectID.UUID)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChatNotFound
			}
			if err != nil {
				return xerrors.Errorf("check root chat project: %w", err)
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

// SetFamilyProjectInput configures [SetFamilyProject].
type SetFamilyProjectInput struct {
	// RootID identifies the family root. SetFamilyProject rejects
	// calls for child chats with [ErrChatNotRoot] and unknown chats,
	// or chats in a deleted project, with [ErrChatNotFound].
	RootID uuid.UUID
	// ProjectID is the family's new project. NULL removes the family
	// from its project. A missing or deleted project returns
	// [ErrChatProjectNotFound].
	ProjectID uuid.NullUUID
}

// SetFamilyProject moves a root chat into a project, or out of its
// project, inside one transaction. Every family member must be in W,
// E0 or E1, or the move rolls back, so no step of the family runs with
// the old project's memory tools after the move. Subagent chats move with
// the root, so a project delete archives the whole family.
//
// Like [SetFamilyArchived], it locks the root first and then each
// member in GetChatFamilyIDsByRootID order, and it publishes only after
// the commit. On success it returns one [database.Chat] per family
// member, root first. A root already in the project changes nothing and
// returns no chats; the check reads the locked row, so a move back to
// the current project waits for a move still in flight.
func SetFamilyProject(
	ctx context.Context,
	store database.Store,
	publisher Publisher,
	input SetFamilyProjectInput,
) ([]database.Chat, error) {
	if store == nil {
		return nil, xerrors.New("chatstate: SetFamilyProject called with nil store")
	}
	if publisher == nil {
		return nil, xerrors.New("chatstate: SetFamilyProject called with nil publisher")
	}

	buffer := NewPublishBuffer(publisher)
	defer buffer.Discard()

	var familyChats []database.Chat
	err := store.InTx(func(tx database.Store) error {
		// Child creation takes FOR SHARE on the root row, so holding this
		// lock keeps new subagent chats out of the family during the move.
		root, err := tx.GetChatByIDForUpdate(ctx, input.RootID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChatNotFound
			}
			return xerrors.Errorf("lock root chat for move: %w", err)
		}
		if root.ParentChatID.Valid {
			return ErrChatNotRoot
		}
		if root.ProjectID == input.ProjectID {
			return nil
		}
		// Held until commit, so a family cannot leave a deleted project
		// before its cleanup archives it.
		if root.ProjectID.Valid {
			//nolint:gocritic // The caller authorized the root chat, not its project.
			_, err := tx.GetChatProjectByIDForShare(dbauthz.AsChatd(ctx), root.ProjectID.UUID)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChatNotFound
			}
			if err != nil {
				return xerrors.Errorf("check current chat project: %w", err)
			}
		}
		if input.ProjectID.Valid {
			// Held until commit; see GetChatProjectByIDForShare.
			//nolint:gocritic // The caller authorized the target project.
			if _, err := tx.GetChatProjectByIDForShare(dbauthz.AsChatd(ctx), input.ProjectID.UUID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return ErrChatProjectNotFound
				}
				return xerrors.Errorf("lock target chat project: %w", err)
			}
		}
		ids, err := tx.GetChatFamilyIDsByRootID(ctx, input.RootID)
		if err != nil {
			return xerrors.Errorf("get chat family: %w", err)
		}
		familyChats = make([]database.Chat, 0, len(ids))
		for _, id := range ids {
			var chat database.Chat
			machine := NewChatMachine(tx, buffer, id)
			err := machine.Update(ctx, func(state *Tx, _ database.Store) error {
				result, err := state.SetProject(SetProjectInput{ProjectID: input.ProjectID})
				if err != nil {
					return err
				}
				chat = result.Chat
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

// ArchiveDeletedChatProject archives the unarchived chats of a deleted
// project, sub-chats included, and returns them as archived, along with
// every chat in the project read in the same transaction. ctx must allow
// reading and updating every chat, as chatd does. Each chat moves from any
// execution state to [StateXW], or [StateXE0] from error, with no worker or
// runner and an empty queue. A chat:update per archived chat is published
// after commit so its runner stops; if that fails, the committed result is
// returned with [ErrChatUpdatePublishFailed]. A live project archives
// nothing.
func ArchiveDeletedChatProject(ctx context.Context, store database.Store, publisher Publisher, projectID uuid.UUID) (archived []database.Chat, projectChats []database.Chat, err error) {
	if store == nil {
		return nil, nil, xerrors.New("chatstate: ArchiveDeletedChatProject called with nil store")
	}
	if publisher == nil {
		return nil, nil, xerrors.New("chatstate: ArchiveDeletedChatProject called with nil publisher")
	}
	buffer := NewPublishBuffer(publisher)
	defer buffer.Discard()
	err = store.InTx(func(tx database.Store) error {
		if err := tx.LockChatProjectUnarchivedRootChats(ctx, projectID); err != nil {
			return xerrors.Errorf("lock project root chats: %w", err)
		}
		ids, err := tx.ArchiveChatsOfDeletedChatProject(ctx, projectID)
		if err != nil {
			return xerrors.Errorf("archive project chats: %w", err)
		}
		if len(ids) > 0 {
			if err := tx.DeleteChatQueuedMessagesByChatIDs(ctx, ids); err != nil {
				return xerrors.Errorf("clear project chat queues: %w", err)
			}
			archived, err = tx.GetChatsByIDs(ctx, ids)
			if err != nil {
				return xerrors.Errorf("reload archived project chats: %w", err)
			}
			for _, chat := range archived {
				if err := buffer.Publish(coderdpubsub.ChatStateUpdateChannel(chat.ID), buildChatUpdateMessage(chat)); err != nil {
					return xerrors.Errorf("buffer chat update: %w", err)
				}
			}
		}
		// Read here, not after commit, so a failure cannot lose the watch
		// events: the sweep only retries projects with unarchived chats.
		projectChats, err = tx.GetChatsByProjectID(ctx, projectID)
		if err != nil {
			return xerrors.Errorf("list project chats: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := buffer.Flush(); err != nil {
		return archived, projectChats, errors.Join(ErrChatUpdatePublishFailed, err)
	}
	return archived, projectChats, nil
}
