package example

import (
	"context"
	"database/sql"

	"github.com/coder/coder/v2/coderd/database"
)

func writesThroughStore(ctx context.Context, store database.Store) error {
	_, err := store.InsertChatMessages(ctx, database.InsertChatMessagesParams{}) // want `InsertChatMessages writes chat_messages outside a chatstate transition`
	if err != nil {
		return err
	}
	return store.SoftDeleteChatMessageByID(ctx, 1) // want `SoftDeleteChatMessageByID writes chat_messages outside a chatstate transition`
}

func readsThroughStore(ctx context.Context, store database.Store) error {
	_, err := store.GetChatMessagesByChatID(ctx, 1)
	return err
}

func writesRawSQL(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx /* want `SQL writes chat_messages outside a chatstate transition` */, `
		UPDATE chat_messages
		SET content = $2
		WHERE id = $1
	`, 1, "x")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "INSERT INTO chat_messages (chat_id) VALUES ($1)", 1) // want `SQL writes chat_messages outside a chatstate transition`
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "DELETE FROM public.chat_messages WHERE id = $1", 1) // want `SQL writes chat_messages outside a chatstate transition`
	return err
}

func readsRawSQL(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, "SELECT count(*) FROM chat_messages WHERE chat_id = $1", 1)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "UPDATE chats SET title = $2 FROM chat_messages WHERE chats.id = $1", 1, "t")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "INSERT INTO chat_messages_archive (id) VALUES ($1)", 1)
	return err
}

type unrelated struct{}

func (unrelated) InsertChatMessages(context.Context, database.InsertChatMessagesParams) ([]database.ChatMessage, error) {
	return nil, nil
}

// A method of the same name on a type outside the database package is not
// a store write.
func unrelatedMethod(ctx context.Context) error {
	_, err := unrelated{}.InsertChatMessages(ctx, database.InsertChatMessagesParams{})
	return err
}
