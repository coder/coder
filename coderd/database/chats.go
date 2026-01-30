// Code generated manually for chat tables. Re-generate with sqlc once the
// pre-existing workspaces.sql ambiguity bug is fixed.

package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const deleteChatByID = `-- name: DeleteChatByID :exec
DELETE FROM chats WHERE id = $1
`

func (q *sqlQuerier) DeleteChatByID(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.ExecContext(ctx, deleteChatByID, id)
	return err
}

const deleteChatMessagesByChatID = `-- name: DeleteChatMessagesByChatID :exec
DELETE FROM chat_messages WHERE chat_id = $1
`

func (q *sqlQuerier) DeleteChatMessagesByChatID(ctx context.Context, chatID uuid.UUID) error {
	_, err := q.db.ExecContext(ctx, deleteChatMessagesByChatID, chatID)
	return err
}

const getChatByID = `-- name: GetChatByID :one
SELECT id, owner_id, workspace_id, workspace_agent_id, title, status, model_config, worker_id, started_at, created_at, updated_at FROM chats WHERE id = $1
`

func (q *sqlQuerier) GetChatByID(ctx context.Context, id uuid.UUID) (Chat, error) {
	row := q.db.QueryRowContext(ctx, getChatByID, id)
	var i Chat
	err := row.Scan(
		&i.ID,
		&i.OwnerID,
		&i.WorkspaceID,
		&i.WorkspaceAgentID,
		&i.Title,
		&i.Status,
		&i.ModelConfig,
		&i.WorkerID,
		&i.StartedAt,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const getChatMessageByID = `-- name: GetChatMessageByID :one
SELECT id, chat_id, created_at, role, content, tool_calls, tool_call_id, thinking, hidden FROM chat_messages WHERE id = $1
`

func (q *sqlQuerier) GetChatMessageByID(ctx context.Context, id int64) (ChatMessage, error) {
	row := q.db.QueryRowContext(ctx, getChatMessageByID, id)
	var i ChatMessage
	err := row.Scan(
		&i.ID,
		&i.ChatID,
		&i.CreatedAt,
		&i.Role,
		&i.Content,
		&i.ToolCalls,
		&i.ToolCallID,
		&i.Thinking,
		&i.Hidden,
	)
	return i, err
}

const getChatMessagesByChatID = `-- name: GetChatMessagesByChatID :many
SELECT id, chat_id, created_at, role, content, tool_calls, tool_call_id, thinking, hidden FROM chat_messages
WHERE chat_id = $1
ORDER BY created_at ASC
`

func (q *sqlQuerier) GetChatMessagesByChatID(ctx context.Context, chatID uuid.UUID) ([]ChatMessage, error) {
	rows, err := q.db.QueryContext(ctx, getChatMessagesByChatID, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ChatMessage
	for rows.Next() {
		var i ChatMessage
		if err := rows.Scan(
			&i.ID,
			&i.ChatID,
			&i.CreatedAt,
			&i.Role,
			&i.Content,
			&i.ToolCalls,
			&i.ToolCallID,
			&i.Thinking,
			&i.Hidden,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getChatsByOwnerID = `-- name: GetChatsByOwnerID :many
SELECT id, owner_id, workspace_id, workspace_agent_id, title, status, model_config, worker_id, started_at, created_at, updated_at FROM chats
WHERE owner_id = $1
ORDER BY updated_at DESC
`

func (q *sqlQuerier) GetChatsByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]Chat, error) {
	rows, err := q.db.QueryContext(ctx, getChatsByOwnerID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Chat
	for rows.Next() {
		var i Chat
		if err := rows.Scan(
			&i.ID,
			&i.OwnerID,
			&i.WorkspaceID,
			&i.WorkspaceAgentID,
			&i.Title,
			&i.Status,
			&i.ModelConfig,
			&i.WorkerID,
			&i.StartedAt,
			&i.CreatedAt,
			&i.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const insertChat = `-- name: InsertChat :one
INSERT INTO chats (owner_id, workspace_id, workspace_agent_id, title, model_config)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, owner_id, workspace_id, workspace_agent_id, title, status, model_config, worker_id, started_at, created_at, updated_at
`

type InsertChatParams struct {
	OwnerID          uuid.UUID       `db:"owner_id" json:"owner_id"`
	WorkspaceID      uuid.NullUUID   `db:"workspace_id" json:"workspace_id"`
	WorkspaceAgentID uuid.NullUUID   `db:"workspace_agent_id" json:"workspace_agent_id"`
	Title            string          `db:"title" json:"title"`
	ModelConfig      json.RawMessage `db:"model_config" json:"model_config"`
}

func (q *sqlQuerier) InsertChat(ctx context.Context, arg InsertChatParams) (Chat, error) {
	row := q.db.QueryRowContext(ctx, insertChat,
		arg.OwnerID,
		arg.WorkspaceID,
		arg.WorkspaceAgentID,
		arg.Title,
		arg.ModelConfig,
	)
	var i Chat
	err := row.Scan(
		&i.ID,
		&i.OwnerID,
		&i.WorkspaceID,
		&i.WorkspaceAgentID,
		&i.Title,
		&i.Status,
		&i.ModelConfig,
		&i.WorkerID,
		&i.StartedAt,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const insertChatMessage = `-- name: InsertChatMessage :one
INSERT INTO chat_messages (chat_id, role, content, tool_calls, tool_call_id, thinking, hidden)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, chat_id, created_at, role, content, tool_calls, tool_call_id, thinking, hidden
`

type InsertChatMessageParams struct {
	ChatID     uuid.UUID       `db:"chat_id" json:"chat_id"`
	Role       string          `db:"role" json:"role"`
	Content    json.RawMessage `db:"content" json:"content"`
	ToolCalls  json.RawMessage `db:"tool_calls" json:"tool_calls"`
	ToolCallID sql.NullString  `db:"tool_call_id" json:"tool_call_id"`
	Thinking   sql.NullString  `db:"thinking" json:"thinking"`
	Hidden     bool            `db:"hidden" json:"hidden"`
}

func (q *sqlQuerier) InsertChatMessage(ctx context.Context, arg InsertChatMessageParams) (ChatMessage, error) {
	row := q.db.QueryRowContext(ctx, insertChatMessage,
		arg.ChatID,
		arg.Role,
		arg.Content,
		arg.ToolCalls,
		arg.ToolCallID,
		arg.Thinking,
		arg.Hidden,
	)
	var i ChatMessage
	err := row.Scan(
		&i.ID,
		&i.ChatID,
		&i.CreatedAt,
		&i.Role,
		&i.Content,
		&i.ToolCalls,
		&i.ToolCallID,
		&i.Thinking,
		&i.Hidden,
	)
	return i, err
}

const updateChatByID = `-- name: UpdateChatByID :one
UPDATE chats
SET title = $1, updated_at = NOW()
WHERE id = $2
RETURNING id, owner_id, workspace_id, workspace_agent_id, title, status, model_config, worker_id, started_at, created_at, updated_at
`

type UpdateChatByIDParams struct {
	Title string    `db:"title" json:"title"`
	ID    uuid.UUID `db:"id" json:"id"`
}

func (q *sqlQuerier) UpdateChatByID(ctx context.Context, arg UpdateChatByIDParams) (Chat, error) {
	row := q.db.QueryRowContext(ctx, updateChatByID, arg.Title, arg.ID)
	var i Chat
	err := row.Scan(
		&i.ID,
		&i.OwnerID,
		&i.WorkspaceID,
		&i.WorkspaceAgentID,
		&i.Title,
		&i.Status,
		&i.ModelConfig,
		&i.WorkerID,
		&i.StartedAt,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const acquireChat = `-- name: AcquireChat :one
-- Acquires a pending chat for processing. Uses SKIP LOCKED to prevent
-- multiple replicas from acquiring the same chat.
UPDATE chats
SET status = 'running', started_at = $1, updated_at = $1, worker_id = $2
WHERE id = (
    SELECT id FROM chats
    WHERE status = 'pending'
    ORDER BY updated_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, owner_id, workspace_id, workspace_agent_id, title, status, model_config, worker_id, started_at, created_at, updated_at
`

type AcquireChatParams struct {
	StartedAt time.Time `db:"started_at" json:"started_at"`
	WorkerID  uuid.UUID `db:"worker_id" json:"worker_id"`
}

func (q *sqlQuerier) AcquireChat(ctx context.Context, arg AcquireChatParams) (Chat, error) {
	row := q.db.QueryRowContext(ctx, acquireChat, arg.StartedAt, arg.WorkerID)
	var i Chat
	err := row.Scan(
		&i.ID,
		&i.OwnerID,
		&i.WorkspaceID,
		&i.WorkspaceAgentID,
		&i.Title,
		&i.Status,
		&i.ModelConfig,
		&i.WorkerID,
		&i.StartedAt,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const updateChatStatus = `-- name: UpdateChatStatus :one
UPDATE chats
SET status = $1, worker_id = $2, started_at = $3, updated_at = NOW()
WHERE id = $4
RETURNING id, owner_id, workspace_id, workspace_agent_id, title, status, model_config, worker_id, started_at, created_at, updated_at
`

type UpdateChatStatusParams struct {
	Status    ChatStatus    `db:"status" json:"status"`
	WorkerID  uuid.NullUUID `db:"worker_id" json:"worker_id"`
	StartedAt sql.NullTime  `db:"started_at" json:"started_at"`
	ID        uuid.UUID     `db:"id" json:"id"`
}

func (q *sqlQuerier) UpdateChatStatus(ctx context.Context, arg UpdateChatStatusParams) (Chat, error) {
	row := q.db.QueryRowContext(ctx, updateChatStatus, arg.Status, arg.WorkerID, arg.StartedAt, arg.ID)
	var i Chat
	err := row.Scan(
		&i.ID,
		&i.OwnerID,
		&i.WorkspaceID,
		&i.WorkspaceAgentID,
		&i.Title,
		&i.Status,
		&i.ModelConfig,
		&i.WorkerID,
		&i.StartedAt,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const getStaleChats = `-- name: GetStaleChats :many
-- Find chats that appear stuck (running but no heartbeat).
-- Used for recovery after coderd crashes.
SELECT id, owner_id, workspace_id, workspace_agent_id, title, status, model_config, worker_id, started_at, created_at, updated_at
FROM chats
WHERE status = 'running' AND started_at < $1
`

func (q *sqlQuerier) GetStaleChats(ctx context.Context, staleThreshold time.Time) ([]Chat, error) {
	rows, err := q.db.QueryContext(ctx, getStaleChats, staleThreshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Chat
	for rows.Next() {
		var i Chat
		if err := rows.Scan(
			&i.ID,
			&i.OwnerID,
			&i.WorkspaceID,
			&i.WorkspaceAgentID,
			&i.Title,
			&i.Status,
			&i.ModelConfig,
			&i.WorkerID,
			&i.StartedAt,
			&i.CreatedAt,
			&i.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
