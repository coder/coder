package chatstate

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// LinkFiles links files, returning [ErrChatFileCapExceeded] for cap rejections
// and [ErrChatFileUnavailable] for missing files. Use the caller's transaction
// so failures roll back related writes; existing links use no additional slots.
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
		wrapped := xerrors.Errorf("link chat files: %w", err)
		if database.IsForeignKeyViolation(err, database.ForeignKeyChatFileLinksFileID) {
			return errors.Join(ErrChatFileUnavailable, wrapped)
		}
		return wrapped
	}
	if rejected > 0 {
		return ErrChatFileCapExceeded
	}
	return nil
}
