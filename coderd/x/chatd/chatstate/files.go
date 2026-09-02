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
// and [ErrChatFileUnavailable] for files that are missing or attached to
// another chat. Use the caller's transaction so failures roll back related
// writes; existing links use no additional slots. maxLinks is the number of
// most recent files the chat keeps; a non-positive value uses
// [codersdk.DefaultChatMaxAttachmentsPerChat].
func LinkFiles(ctx context.Context, store database.Store, chatID uuid.UUID, fileIDs []uuid.UUID, maxLinks int) error {
	if len(fileIDs) == 0 {
		return nil
	}
	if maxLinks <= 0 {
		maxLinks = codersdk.DefaultChatMaxAttachmentsPerChat
	}
	rejected, err := store.LinkChatFiles(ctx, database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(maxLinks), //nolint:gosec // Deployment validation bounds the limit to the int32 range.
		FileIds:      fileIDs,
	})
	if err != nil {
		wrapped := xerrors.Errorf("link chat files: %w", err)
		if database.IsForeignKeyViolation(err, database.ForeignKeyChatFileLinksFileID) ||
			database.IsUniqueViolation(err, database.UniqueChatFileLinksFileIDKey) {
			return errors.Join(ErrChatFileUnavailable, wrapped)
		}
		return wrapped
	}
	if rejected > 0 {
		return ErrChatFileCapExceeded
	}
	return nil
}
