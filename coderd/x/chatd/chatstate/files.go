package chatstate

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// LinkFiles links files without exceeding the chat attachment cap.
// Call it in the message transaction so link failures roll back the message.
// Existing links do not consume additional cap slots.
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
