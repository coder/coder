package database

import "context"

type ChatMessage struct{}

type InsertChatMessagesParams struct{}

type Store interface {
	InsertChatMessages(context.Context, InsertChatMessagesParams) ([]ChatMessage, error)
	SoftDeleteChatMessageByID(context.Context, int64) error
	GetChatMessagesByChatID(context.Context, int64) ([]ChatMessage, error)
}

// The generated query text lives here and must not be reported.
const insertChatMessages = `-- name: InsertChatMessages :many
INSERT INTO chat_messages (chat_id) VALUES ($1)`
