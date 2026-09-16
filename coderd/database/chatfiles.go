package database

import (
	"context"

	"golang.org/x/xerrors"
)

type (
	DeleteOldChatFilesParams = GetOldUnlinkedChatFileIDsParams
	LinkChatFilesParams      = LinkChatFilesAfterLockParams
)

func (q *sqlQuerier) LinkChatFiles(ctx context.Context, arg LinkChatFilesParams) (int32, error) {
	var rejected int32
	err := q.InTx(func(tx Store) error {
		if _, err := tx.LockChatByID(ctx, arg.ChatID); err != nil {
			return xerrors.Errorf("lock chat: %w", err)
		}

		var err error
		rejected, err = tx.LinkChatFilesAfterLock(ctx, arg)
		if err != nil {
			return xerrors.Errorf("link chat files after lock: %w", err)
		}
		return nil
	}, DefaultTXOptions().WithID("link_chat_files"))
	if err != nil {
		return 0, err
	}
	return rejected, nil
}

func (q *sqlQuerier) DeleteOldChatFiles(ctx context.Context, arg DeleteOldChatFilesParams) (int64, error) {
	// Recheck candidates because links may commit during row-lock waits.
	var deleted int64
	err := q.InTx(func(tx Store) error {
		ids, err := tx.GetOldUnlinkedChatFileIDs(ctx, arg)
		if err != nil {
			return xerrors.Errorf("get old unlinked chat files: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}

		deleted, err = tx.DeleteUnlinkedChatFilesByIDs(ctx, DeleteUnlinkedChatFilesByIDsParams{
			IDs:        ids,
			BeforeTime: arg.BeforeTime,
		})
		if err != nil {
			return xerrors.Errorf("delete old unlinked chat files: %w", err)
		}
		return nil
	}, DefaultTXOptions().WithID("delete_old_chat_files"))
	if err != nil {
		return 0, err
	}
	return deleted, nil
}
