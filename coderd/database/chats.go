// Code generated manually for chat tables. Re-generate with sqlc once the
// pre-existing workspaces.sql ambiguity bug is fixed.

package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/sqlc-dev/pqtype"
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
SELECT id, chat_id, created_at, role, content, tool_call_id, thinking, hidden FROM chat_messages WHERE id = $1
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
		&i.ToolCallID,
		&i.Thinking,
		&i.Hidden,
	)
	return i, err
}

const getChatMessagesByChatID = `-- name: GetChatMessagesByChatID :many
SELECT id, chat_id, created_at, role, content, tool_call_id, thinking, hidden FROM chat_messages
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
INSERT INTO chat_messages (chat_id, role, content, tool_call_id, thinking, hidden)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, chat_id, created_at, role, content, tool_call_id, thinking, hidden
`

type InsertChatMessageParams struct {
	ChatID     uuid.UUID             `db:"chat_id" json:"chat_id"`
	Role       string                `db:"role" json:"role"`
	Content    pqtype.NullRawMessage `db:"content" json:"content"`
	ToolCallID sql.NullString        `db:"tool_call_id" json:"tool_call_id"`
	Thinking   sql.NullString        `db:"thinking" json:"thinking"`
	Hidden     bool                  `db:"hidden" json:"hidden"`
}

func (q *sqlQuerier) InsertChatMessage(ctx context.Context, arg InsertChatMessageParams) (ChatMessage, error) {
	row := q.db.QueryRowContext(ctx, insertChatMessage,
		arg.ChatID,
		arg.Role,
		arg.Content,
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

const updateChatWorkspace = `-- name: UpdateChatWorkspace :one
UPDATE chats
SET workspace_id = $1, workspace_agent_id = $2, updated_at = NOW()
WHERE id = $3
RETURNING id, owner_id, workspace_id, workspace_agent_id, title, status, model_config, worker_id, started_at, created_at, updated_at
`

type UpdateChatWorkspaceParams struct {
	WorkspaceID      uuid.NullUUID `db:"workspace_id" json:"workspace_id"`
	WorkspaceAgentID uuid.NullUUID `db:"workspace_agent_id" json:"workspace_agent_id"`
	ID               uuid.UUID     `db:"id" json:"id"`
}

func (q *sqlQuerier) UpdateChatWorkspace(ctx context.Context, arg UpdateChatWorkspaceParams) (Chat, error) {
	row := q.db.QueryRowContext(ctx, updateChatWorkspace, arg.WorkspaceID, arg.WorkspaceAgentID, arg.ID)
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

const getChatDiffStatusByChatID = `-- name: GetChatDiffStatusByChatID :one
SELECT chat_id, github_pr_url, pull_request_state, pull_request_open, changes_requested, additions, deletions, changed_files, refreshed_at, stale_at, created_at, updated_at
FROM chat_diff_statuses
WHERE chat_id = $1
`

func (q *sqlQuerier) GetChatDiffStatusByChatID(ctx context.Context, chatID uuid.UUID) (ChatDiffStatus, error) {
	row := q.db.QueryRowContext(ctx, getChatDiffStatusByChatID, chatID)
	var i ChatDiffStatus
	err := row.Scan(
		&i.ChatID,
		&i.GithubPrUrl,
		&i.PullRequestState,
		&i.PullRequestOpen,
		&i.ChangesRequested,
		&i.Additions,
		&i.Deletions,
		&i.ChangedFiles,
		&i.RefreshedAt,
		&i.StaleAt,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const getChatDiffStatusesByChatIDs = `-- name: GetChatDiffStatusesByChatIDs :many
SELECT chat_id, github_pr_url, pull_request_state, pull_request_open, changes_requested, additions, deletions, changed_files, refreshed_at, stale_at, created_at, updated_at
FROM chat_diff_statuses
WHERE chat_id = ANY($1::uuid[])
`

func (q *sqlQuerier) GetChatDiffStatusesByChatIDs(ctx context.Context, chatIDs []uuid.UUID) ([]ChatDiffStatus, error) {
	if len(chatIDs) == 0 {
		return []ChatDiffStatus{}, nil
	}

	rows, err := q.db.QueryContext(ctx, getChatDiffStatusesByChatIDs, pq.Array(chatIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ChatDiffStatus
	for rows.Next() {
		var i ChatDiffStatus
		if err := rows.Scan(
			&i.ChatID,
			&i.GithubPrUrl,
			&i.PullRequestState,
			&i.PullRequestOpen,
			&i.ChangesRequested,
			&i.Additions,
			&i.Deletions,
			&i.ChangedFiles,
			&i.RefreshedAt,
			&i.StaleAt,
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

const upsertChatDiffStatusReference = `-- name: UpsertChatDiffStatusReference :one
INSERT INTO chat_diff_statuses (
	chat_id,
	github_pr_url,
	stale_at
) VALUES (
	$1,
	$2,
	$3
)
ON CONFLICT (chat_id) DO UPDATE SET
	github_pr_url = EXCLUDED.github_pr_url,
	stale_at = EXCLUDED.stale_at,
	updated_at = NOW()
RETURNING chat_id, github_pr_url, pull_request_state, pull_request_open, changes_requested, additions, deletions, changed_files, refreshed_at, stale_at, created_at, updated_at
`

type UpsertChatDiffStatusReferenceParams struct {
	ChatID      uuid.UUID      `db:"chat_id" json:"chat_id"`
	GithubPrUrl sql.NullString `db:"github_pr_url" json:"github_pr_url"`
	StaleAt     time.Time      `db:"stale_at" json:"stale_at"`
}

func (q *sqlQuerier) UpsertChatDiffStatusReference(ctx context.Context, arg UpsertChatDiffStatusReferenceParams) (ChatDiffStatus, error) {
	row := q.db.QueryRowContext(ctx, upsertChatDiffStatusReference, arg.ChatID, arg.GithubPrUrl, arg.StaleAt)
	var i ChatDiffStatus
	err := row.Scan(
		&i.ChatID,
		&i.GithubPrUrl,
		&i.PullRequestState,
		&i.PullRequestOpen,
		&i.ChangesRequested,
		&i.Additions,
		&i.Deletions,
		&i.ChangedFiles,
		&i.RefreshedAt,
		&i.StaleAt,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const upsertChatDiffStatus = `-- name: UpsertChatDiffStatus :one
INSERT INTO chat_diff_statuses (
	chat_id,
	github_pr_url,
	pull_request_state,
	pull_request_open,
	changes_requested,
	additions,
	deletions,
	changed_files,
	refreshed_at,
	stale_at
) VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7,
	$8,
	$9,
	$10
)
ON CONFLICT (chat_id) DO UPDATE SET
	github_pr_url = EXCLUDED.github_pr_url,
	pull_request_state = EXCLUDED.pull_request_state,
	pull_request_open = EXCLUDED.pull_request_open,
	changes_requested = EXCLUDED.changes_requested,
	additions = EXCLUDED.additions,
	deletions = EXCLUDED.deletions,
	changed_files = EXCLUDED.changed_files,
	refreshed_at = EXCLUDED.refreshed_at,
	stale_at = EXCLUDED.stale_at,
	updated_at = NOW()
RETURNING chat_id, github_pr_url, pull_request_state, pull_request_open, changes_requested, additions, deletions, changed_files, refreshed_at, stale_at, created_at, updated_at
`

type UpsertChatDiffStatusParams struct {
	ChatID           uuid.UUID      `db:"chat_id" json:"chat_id"`
	GithubPrUrl      sql.NullString `db:"github_pr_url" json:"github_pr_url"`
	PullRequestState string         `db:"pull_request_state" json:"pull_request_state"`
	PullRequestOpen  bool           `db:"pull_request_open" json:"pull_request_open"`
	ChangesRequested bool           `db:"changes_requested" json:"changes_requested"`
	Additions        int32          `db:"additions" json:"additions"`
	Deletions        int32          `db:"deletions" json:"deletions"`
	ChangedFiles     int32          `db:"changed_files" json:"changed_files"`
	RefreshedAt      time.Time      `db:"refreshed_at" json:"refreshed_at"`
	StaleAt          time.Time      `db:"stale_at" json:"stale_at"`
}

func (q *sqlQuerier) UpsertChatDiffStatus(ctx context.Context, arg UpsertChatDiffStatusParams) (ChatDiffStatus, error) {
	row := q.db.QueryRowContext(ctx, upsertChatDiffStatus,
		arg.ChatID,
		arg.GithubPrUrl,
		arg.PullRequestState,
		arg.PullRequestOpen,
		arg.ChangesRequested,
		arg.Additions,
		arg.Deletions,
		arg.ChangedFiles,
		arg.RefreshedAt,
		arg.StaleAt,
	)
	var i ChatDiffStatus
	err := row.Scan(
		&i.ChatID,
		&i.GithubPrUrl,
		&i.PullRequestState,
		&i.PullRequestOpen,
		&i.ChangesRequested,
		&i.Additions,
		&i.Deletions,
		&i.ChangedFiles,
		&i.RefreshedAt,
		&i.StaleAt,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}
