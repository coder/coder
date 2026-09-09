package example

import (
	"context"
	"database/sql"

	"github.com/coder/coder/v2/coderd/database"
)

// Tests seed and mutate chat_messages directly on purpose and are not
// reported.
func seedForTest(ctx context.Context, store database.Store, db *sql.DB) error {
	if _, err := store.InsertChatMessages(ctx, database.InsertChatMessagesParams{}); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, "UPDATE chat_messages SET content = $2 WHERE id = $1", 1, "x")
	return err
}
