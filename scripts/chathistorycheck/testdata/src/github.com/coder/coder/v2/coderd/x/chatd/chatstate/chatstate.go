package chatstate

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
)

// chatstate owns the transitions, so its writes are not reported.
func insertMessages(ctx context.Context, store database.Store) error {
	_, err := store.InsertChatMessages(ctx, database.InsertChatMessagesParams{})
	return err
}
