-- name: DeleteChatByID :exec
DELETE FROM
    chats
WHERE
    id = @id::uuid;

-- name: DeleteChatMessagesByChatID :exec
DELETE FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid;

-- name: GetChatByID :one
SELECT
    *
FROM
    chats
WHERE
    id = @id::uuid;

-- name: GetChatMessageByID :one
SELECT
    *
FROM
    chat_messages
WHERE
    id = @id::bigint;

-- name: GetChatMessagesByChatID :many
SELECT
    *
FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid
ORDER BY
    created_at ASC;

-- name: GetChatsByOwnerID :many
SELECT
    *
FROM
    chats
WHERE
    owner_id = @owner_id::uuid
ORDER BY
    updated_at DESC;

-- name: ListChildChatsByParentID :many
SELECT
    *
FROM
    chats
WHERE
    parent_chat_id = @parent_chat_id::uuid
ORDER BY
    created_at ASC;

-- name: ListChatsByRootID :many
SELECT
    *
FROM
    chats
WHERE
    root_chat_id = @root_chat_id::uuid
ORDER BY
    created_at ASC;

-- name: InsertChat :one
INSERT INTO chats (
    owner_id,
    workspace_id,
    workspace_agent_id,
    parent_chat_id,
    root_chat_id,
    task_status,
    title,
    model_config
) VALUES (
    @owner_id::uuid,
    sqlc.narg('workspace_id')::uuid,
    sqlc.narg('workspace_agent_id')::uuid,
    sqlc.narg('parent_chat_id')::uuid,
    sqlc.narg('root_chat_id')::uuid,
    COALESCE(sqlc.narg('task_status')::chat_task_status, 'reported'::chat_task_status),
    @title::text,
    @model_config::jsonb
)
RETURNING
    *;

-- name: InsertChatMessage :one
INSERT INTO chat_messages (
    chat_id,
    role,
    content,
    tool_call_id,
    thinking,
    hidden
) VALUES (
    @chat_id::uuid,
    @role::text,
    sqlc.narg('content')::jsonb,
    sqlc.narg('tool_call_id')::text,
    sqlc.narg('thinking')::text,
    @hidden::boolean
)
RETURNING
    *;

-- name: UpdateChatByID :one
UPDATE
    chats
SET
    title = @title::text,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: UpdateChatWorkspace :one
UPDATE
    chats
SET
    workspace_id = sqlc.narg('workspace_id')::uuid,
    workspace_agent_id = sqlc.narg('workspace_agent_id')::uuid,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: AcquireChat :one
-- Acquires a pending chat for processing. Uses SKIP LOCKED to prevent
-- multiple replicas from acquiring the same chat.
UPDATE
    chats
SET
    status = 'running'::chat_status,
    started_at = @started_at::timestamptz,
    updated_at = @started_at::timestamptz,
    worker_id = @worker_id::uuid
WHERE
    id = (
        SELECT
            id
        FROM
            chats
        WHERE
            status = 'pending'::chat_status
        ORDER BY
            updated_at ASC
        FOR UPDATE
            SKIP LOCKED
        LIMIT
            1
    )
RETURNING
    *;

-- name: UpdateChatStatus :one
UPDATE
    chats
SET
    status = @status::chat_status,
    worker_id = sqlc.narg('worker_id')::uuid,
    started_at = sqlc.narg('started_at')::timestamptz,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: UpdateChatTaskStatus :one
UPDATE
    chats
SET
    task_status = @task_status::chat_task_status,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: UpdateChatTaskReport :one
UPDATE
    chats
SET
    task_report = sqlc.narg('task_report')::text,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: GetStaleChats :many
-- Find chats that appear stuck (running but no heartbeat).
-- Used for recovery after coderd crashes.
SELECT
    *
FROM
    chats
WHERE
    status = 'running'::chat_status
    AND started_at < @stale_threshold::timestamptz;

-- name: GetChatDiffStatusByChatID :one
SELECT
    *
FROM
    chat_diff_statuses
WHERE
    chat_id = @chat_id::uuid;

-- name: GetChatDiffStatusesByChatIDs :many
SELECT
    *
FROM
    chat_diff_statuses
WHERE
    chat_id = ANY(@chat_ids::uuid[]);

-- name: UpsertChatDiffStatusReference :one
INSERT INTO chat_diff_statuses (
    chat_id,
    url,
    git_branch,
    git_remote_origin,
    stale_at
) VALUES (
    @chat_id::uuid,
    sqlc.narg('url')::text,
    @git_branch::text,
    @git_remote_origin::text,
    @stale_at::timestamptz
)
ON CONFLICT (chat_id) DO UPDATE
SET
    url = CASE
        WHEN EXCLUDED.url IS NOT NULL THEN EXCLUDED.url
        ELSE chat_diff_statuses.url
    END,
    git_branch = CASE
        WHEN EXCLUDED.git_branch != '' THEN EXCLUDED.git_branch
        ELSE chat_diff_statuses.git_branch
    END,
    git_remote_origin = CASE
        WHEN EXCLUDED.git_remote_origin != '' THEN EXCLUDED.git_remote_origin
        ELSE chat_diff_statuses.git_remote_origin
    END,
    stale_at = EXCLUDED.stale_at,
    updated_at = NOW()
RETURNING
    *;

-- name: UpsertChatDiffStatus :one
INSERT INTO chat_diff_statuses (
    chat_id,
    url,
    pull_request_state,
    changes_requested,
    additions,
    deletions,
    changed_files,
    refreshed_at,
    stale_at
) VALUES (
    @chat_id::uuid,
    sqlc.narg('url')::text,
    sqlc.narg('pull_request_state')::text,
    @changes_requested::boolean,
    @additions::integer,
    @deletions::integer,
    @changed_files::integer,
    @refreshed_at::timestamptz,
    @stale_at::timestamptz
)
ON CONFLICT (chat_id) DO UPDATE
SET
    url = EXCLUDED.url,
    pull_request_state = EXCLUDED.pull_request_state,
    changes_requested = EXCLUDED.changes_requested,
    additions = EXCLUDED.additions,
    deletions = EXCLUDED.deletions,
    changed_files = EXCLUDED.changed_files,
    refreshed_at = EXCLUDED.refreshed_at,
    stale_at = EXCLUDED.stale_at,
    updated_at = NOW()
RETURNING
    *;
