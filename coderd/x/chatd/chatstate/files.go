package chatstate

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// LinkFiles records chat_file_links rows for the given uploaded files.
// It must run on the same transaction that persists the message content
// referencing the files, so a cap rejection or database error rolls
// back the message too and purge never sees a live message referencing
// an unlinked file. Returns ErrChatFileCapExceeded when the per-chat
// cap (codersdk.MaxChatFileIDs) would be exceeded; already-linked files
// do not count as new.
func LinkFiles(ctx context.Context, store database.Store, chatID uuid.UUID, fileIDs []uuid.UUID) error {
	if len(fileIDs) == 0 {
		return nil
	}
	rejected, err := store.LinkChatFiles(ctx, database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      fileIDs,
	})
	if err != nil {
		return xerrors.Errorf("link chat files: %w", err)
	}
	if rejected > 0 {
		return ErrChatFileCapExceeded
	}
	return nil
}
