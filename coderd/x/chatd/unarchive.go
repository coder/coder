package chatd

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// ErrChildUnarchiveParentArchived is returned by
// UnarchiveChildChatAtomic when the caller attempts to unarchive a
// child chat whose parent is still archived. Callers should
// translate this to a 400 response so the user is told why the
// operation is rejected.
var ErrChildUnarchiveParentArchived = xerrors.New(
	"cannot unarchive child chat while parent is archived",
)

// UnarchiveChildChatAtomic unarchives a child chat only when its
// parent is currently active. The operation is atomic with respect
// to a concurrent ArchiveChatByID cascade on the parent:
//
//  1. GetChatByIDForUpdate(child.ID) takes a row-level lock on the
//     child. A concurrent ArchiveChatByID(parent.ID) must also
//     update the child row (via the root_chat_id cascade predicate)
//     and will serialize behind this lock.
//  2. Once the lock is held, re-read the child and the parent inside
//     the transaction. If the child is already active, return
//     without writing (idempotent recovery path). If the parent is
//     archived, return ErrChildUnarchiveParentArchived.
//  3. Otherwise UnarchiveChatByID(child.ID). The returned rows are
//     the updated child (one row).
//
// Locking the child before reading the parent is deliberate. Locking
// the parent first deadlocks with a concurrent cascade: the cascade
// would take child-row locks in scan order before acquiring the
// parent lock we hold, then wait on us for the parent while we wait
// on it for the child.
//
// Returns an empty slice and nil error when the child is already
// active at lock time (no-op).
func UnarchiveChildChatAtomic(
	ctx context.Context,
	db database.Store,
	child database.Chat,
) ([]database.Chat, error) {
	if child.ID == uuid.Nil {
		return nil, xerrors.New("chat_id is required")
	}
	if !child.ParentChatID.Valid {
		return nil, xerrors.New("chat is not a child chat")
	}

	var updated []database.Chat
	err := db.InTx(func(tx database.Store) error {
		locked, err := tx.GetChatByIDForUpdate(ctx, child.ID)
		if err != nil {
			return xerrors.Errorf("lock child for unarchive: %w", err)
		}
		if !locked.Archived {
			// Already unarchived by a concurrent caller. Treat as
			// a no-op so the handler's response is idempotent.
			return nil
		}
		parent, err := tx.GetChatByID(ctx, child.ParentChatID.UUID)
		if err != nil {
			return xerrors.Errorf("load parent chat: %w", err)
		}
		if parent.Archived {
			return ErrChildUnarchiveParentArchived
		}
		updated, err = tx.UnarchiveChatByID(ctx, child.ID)
		if err != nil {
			return xerrors.Errorf("unarchive child chat: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		// Preserve the sentinel so callers can compare with
		// errors.Is after wrapping.
		if errors.Is(err, ErrChildUnarchiveParentArchived) {
			return nil, ErrChildUnarchiveParentArchived
		}
		return nil, err
	}
	return updated, nil
}
