package chatstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
)

// SetFamilyArchivedInput configures [SetFamilyArchived]. The struct
// shape avoids a boolean flag parameter at the API surface; callers
// build it explicitly with named fields for clarity.
type SetFamilyArchivedInput struct {
	// RootID identifies the family root. SetFamilyArchived rejects
	// calls for child chats with [ErrChatNotRoot] and unknown chats
	// with [ErrChatNotFound]. Unarchiving a root whose project is
	// archived returns [ErrChatProjectArchived].
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
		if !input.Archived && root.ProjectID.Valid {
			if err := requireChatProjectNotArchived(ctx, tx, root.ProjectID.UUID); err != nil {
				return err
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
	// calls for child chats with [ErrChatNotRoot] and unknown chats
	// with [ErrChatNotFound].
	RootID uuid.UUID
	// ProjectID is the root chat's new project. NULL removes the chat
	// from its project. An archived project returns
	// [ErrChatProjectArchived].
	ProjectID uuid.NullUUID
}

// SetFamilyProject moves a root chat into a project, or out of its
// project, inside one transaction. Every family member must be in W,
// E0, E1, E1P or P, or the move rolls back, so no step of the family
// runs with the old project's memory tools after the move. Subagent
// chats have no project and use the root's, so only the root's
// project_id changes.
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
		// The root is locked before the project. A project archive locks
		// the project first and then only chats already in it, and an
		// archived chat cannot move, so the two orders cannot deadlock.
		if input.ProjectID.Valid {
			if err := requireChatProjectNotArchived(ctx, tx, input.ProjectID.UUID); err != nil {
				return err
			}
		}
		ids, err := tx.GetChatFamilyIDsByRootID(ctx, input.RootID)
		if err != nil {
			return xerrors.Errorf("get chat family: %w", err)
		}
		familyChats = make([]database.Chat, 0, len(ids))
		for _, id := range ids {
			projectID := uuid.NullUUID{}
			if id == input.RootID {
				projectID = input.ProjectID
			}
			var chat database.Chat
			machine := NewChatMachine(tx, buffer, id)
			err := machine.Update(ctx, func(state *Tx, _ database.Store) error {
				result, err := state.SetProject(SetProjectInput{ProjectID: projectID})
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

// requireChatProjectNotArchived locks the project FOR SHARE until commit
// and returns [ErrChatProjectArchived] when it is archived. Holding the
// lock serializes the caller with [ArchiveProjectChats], which locks the
// project FOR UPDATE before it archives the project's chats. A missing
// project passes, so the caller's write fails on the project_id foreign
// key as before.
func requireChatProjectNotArchived(ctx context.Context, tx database.Store, projectID uuid.UUID) error {
	//nolint:gocritic // The caller authorized the chat, and its owner may not read the project.
	project, err := tx.GetChatProjectByIDForShare(dbauthz.AsChatd(ctx), projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("lock chat project: %w", err)
	}
	if project.ArchivedAt.Valid {
		return ErrChatProjectArchived
	}
	return nil
}

// chatProjectArchivedStopError is the last_error of a project chat that
// archiving stopped while it had queued messages. It has the shape
// FinishError writes, so clients render it like any terminal error.
const chatProjectArchivedStopError = "Stopped when the project was archived."

// ArchiveProjectChatsInput configures [ArchiveProjectChats].
type ArchiveProjectChatsInput struct {
	ProjectID uuid.UUID
	// StopActiveChats archives the project even when a chat in it is
	// running, interrupting, waiting for tool results, or paused.
	// Without it such chats return a *ChatProjectActiveChatsError.
	StopActiveChats bool
	// OnDeadlockRetry, when set, is called before each retry of a
	// transaction that Postgres aborted to break a deadlock.
	OnDeadlockRetry func(attempt int, err error)
}

// archiveProjectAttempts bounds how often [ArchiveProjectChats] runs its
// transaction when Postgres aborts it to break a deadlock.
const archiveProjectAttempts = 3

// ArchiveProjectChats archives a project and every unarchived chat in it,
// of every owner, subagent chats included, in one transaction. It returns
// the archived project and the chats it archived.
//
// ctx's actor must be allowed to delete the project; the chat writes run
// as chatd because the project's chats can belong to other users.
//
// Each chat moves to XW, or to XE0, XE1 or XE1P when it is in error or
// has queued messages: queued messages are kept, and a non-error chat
// with a queue moves to error. Worker, runner and pending requires-action
// and compaction state are cleared, and a chat:update per archived chat is
// published after commit, so active runners stop and discard their
// partial reply. If that publish fails, the committed result is returned
// with [ErrChatUpdatePublishFailed]. The chats are marked so
// [UnarchiveProjectChats] restores only them; chats archived before stay
// archived.
//
// Locks: the project FOR UPDATE, then the unarchived root chats in id
// order. Chat creation in the project, moves into it, and chat unarchives
// hold the project FOR SHARE, so they either finish first or see the
// project archived. Other writers can lock several of the project's chats
// in another order in one statement, such as a worker's heartbeat over
// its running chats, so the root locks can deadlock with them. Postgres
// then rolls back one transaction; when it rolls back the archive, the
// archive runs again.
func ArchiveProjectChats(
	ctx context.Context,
	store database.Store,
	publisher Publisher,
	input ArchiveProjectChatsInput,
) (database.ChatProject, []database.Chat, error) {
	if store == nil {
		return database.ChatProject{}, nil, xerrors.New("chatstate: ArchiveProjectChats called with nil store")
	}
	if publisher == nil {
		return database.ChatProject{}, nil, xerrors.New("chatstate: ArchiveProjectChats called with nil publisher")
	}
	stoppedError, err := json.Marshal(codersdk.ChatError{Message: chatProjectArchivedStopError})
	if err != nil {
		return database.ChatProject{}, nil, xerrors.Errorf("marshal stopped error: %w", err)
	}

	var (
		project  database.ChatProject
		archived []database.Chat
		buffer   *PublishBuffer
	)
	for attempt := 1; ; attempt++ {
		// Each attempt buffers its own publications, so a rolled back
		// attempt publishes nothing.
		buffer = NewPublishBuffer(publisher)
		project, archived, err = archiveProjectChatsTx(ctx, store, buffer, input, stoppedError)
		if err == nil {
			break
		}
		buffer.Discard()
		if attempt == archiveProjectAttempts || !database.IsDeadlockError(err) {
			return database.ChatProject{}, nil, err
		}
		if input.OnDeadlockRetry != nil {
			input.OnDeadlockRetry(attempt, err)
		}
	}
	if err := buffer.Flush(); err != nil {
		return project, archived, errors.Join(ErrChatUpdatePublishFailed, err)
	}
	return project, archived, nil
}

// archiveProjectChatsTx runs one attempt of [ArchiveProjectChats] and
// buffers the chat:update messages in buffer.
func archiveProjectChatsTx(
	ctx context.Context,
	store database.Store,
	buffer *PublishBuffer,
	input ArchiveProjectChatsInput,
	stoppedError json.RawMessage,
) (database.ChatProject, []database.Chat, error) {
	//nolint:gocritic // The project's chats can belong to other users; ctx authorizes the project.
	chatdCtx := dbauthz.AsChatd(ctx)
	var (
		project  database.ChatProject
		archived []database.Chat
	)
	err := store.InTx(func(tx database.Store) error {
		var err error
		project, err = lockChatProject(ctx, tx, input.ProjectID)
		if err != nil {
			return err
		}
		if project.ArchivedAt.Valid {
			return ErrChatProjectAlreadyArchived
		}
		if err := tx.LockChatProjectUnarchivedRootChats(chatdCtx, input.ProjectID); err != nil {
			return xerrors.Errorf("lock project root chats: %w", err)
		}
		// Counted under the root locks, so a root cannot become active
		// between the check and the archive.
		preview, err := tx.CountChatProjectArchivePreview(chatdCtx, input.ProjectID)
		if err != nil {
			return xerrors.Errorf("count active project chats: %w", err)
		}
		if preview.ActiveChatCount > 0 && !input.StopActiveChats {
			return &ChatProjectActiveChatsError{ActiveChatCount: preview.ActiveChatCount}
		}
		ids, err := tx.ArchiveChatsOfChatProject(chatdCtx, database.ArchiveChatsOfChatProjectParams{
			ProjectID:    input.ProjectID,
			StoppedError: stoppedError,
		})
		if err != nil {
			return xerrors.Errorf("archive project chats: %w", err)
		}
		project, err = tx.UpdateChatProjectArchivedAt(ctx, database.UpdateChatProjectArchivedAtParams{
			ID:         input.ProjectID,
			ArchivedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
		})
		if err != nil {
			return xerrors.Errorf("archive project: %w", err)
		}
		archived, err = reloadAndPublishChats(chatdCtx, tx, buffer, ids)
		return err
	}, nil)
	if err != nil {
		return database.ChatProject{}, nil, err
	}
	return project, archived, nil
}

// UnarchiveProjectChats unarchives a project and the chats that
// [ArchiveProjectChats] archived with it, in one transaction. It returns
// the project and the chats it unarchived. Chats keep their execution
// status, so each lands in W, E0, E1 or E1P. ctx's actor must be allowed
// to delete the project; the chat writes run as chatd. If publishing
// chat:update fails, the committed result is returned with
// [ErrChatUpdatePublishFailed].
func UnarchiveProjectChats(
	ctx context.Context,
	store database.Store,
	publisher Publisher,
	projectID uuid.UUID,
) (database.ChatProject, []database.Chat, error) {
	if store == nil {
		return database.ChatProject{}, nil, xerrors.New("chatstate: UnarchiveProjectChats called with nil store")
	}
	if publisher == nil {
		return database.ChatProject{}, nil, xerrors.New("chatstate: UnarchiveProjectChats called with nil publisher")
	}

	buffer := NewPublishBuffer(publisher)
	defer buffer.Discard()

	//nolint:gocritic // The project's chats can belong to other users; ctx authorizes the project.
	chatdCtx := dbauthz.AsChatd(ctx)
	var (
		project    database.ChatProject
		unarchived []database.Chat
	)
	err := store.InTx(func(tx database.Store) error {
		var err error
		project, err = lockChatProject(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if !project.ArchivedAt.Valid {
			return ErrChatProjectNotArchived
		}
		project, err = tx.UpdateChatProjectArchivedAt(ctx, database.UpdateChatProjectArchivedAtParams{
			ID:         projectID,
			ArchivedAt: sql.NullTime{},
		})
		if err != nil {
			return xerrors.Errorf("unarchive project: %w", err)
		}
		ids, err := tx.UnarchiveChatsOfChatProject(chatdCtx, projectID)
		if err != nil {
			return xerrors.Errorf("unarchive project chats: %w", err)
		}
		unarchived, err = reloadAndPublishChats(chatdCtx, tx, buffer, ids)
		return err
	}, nil)
	if err != nil {
		return database.ChatProject{}, nil, err
	}
	if err := buffer.Flush(); err != nil {
		return project, unarchived, errors.Join(ErrChatUpdatePublishFailed, err)
	}
	return project, unarchived, nil
}

func lockChatProject(ctx context.Context, tx database.Store, projectID uuid.UUID) (database.ChatProject, error) {
	project, err := tx.GetChatProjectByIDForUpdate(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return database.ChatProject{}, ErrChatProjectNotFound
	}
	if err != nil {
		return database.ChatProject{}, xerrors.Errorf("lock chat project: %w", err)
	}
	return project, nil
}

// reloadAndPublishChats reads the chats a bulk update changed and buffers
// a chat:update for each, as ChatMachine.Update does for one chat.
func reloadAndPublishChats(ctx context.Context, tx database.Store, buffer *PublishBuffer, ids []uuid.UUID) ([]database.Chat, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	chats, err := tx.GetChatsByIDsForRunnerSync(ctx, ids)
	if err != nil {
		return nil, xerrors.Errorf("reload project chats: %w", err)
	}
	for _, chat := range chats {
		if err := buffer.Publish(coderdpubsub.ChatStateUpdateChannel(chat.ID), buildChatUpdateMessage(chat)); err != nil {
			return nil, xerrors.Errorf("buffer chat update: %w", err)
		}
	}
	return chats, nil
}
