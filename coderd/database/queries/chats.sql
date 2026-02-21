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

-- name: GetChatMessagesForPromptByChatID :many
WITH latest_compressed_summary AS (
    SELECT
        id
    FROM
        chat_messages
    WHERE
        chat_id = @chat_id::uuid
        AND role = 'system'
        AND hidden = TRUE
        AND compressed = TRUE
    ORDER BY
        created_at DESC,
        id DESC
    LIMIT
        1
)
SELECT
    *
FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid
    AND (
        (
            role = 'system'
            AND hidden = TRUE
            AND compressed = FALSE
        )
        OR (
            compressed = FALSE
            AND (
                NOT EXISTS (
                    SELECT
                        1
                    FROM
                        latest_compressed_summary
                )
                OR id > (
                    SELECT
                        id
                    FROM
                        latest_compressed_summary
                )
            )
        )
        OR id = (
            SELECT
                id
            FROM
                latest_compressed_summary
        )
    )
ORDER BY
    created_at ASC,
    id ASC;

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
    title,
    model_config
) VALUES (
    @owner_id::uuid,
    sqlc.narg('workspace_id')::uuid,
    sqlc.narg('workspace_agent_id')::uuid,
    sqlc.narg('parent_chat_id')::uuid,
    sqlc.narg('root_chat_id')::uuid,
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
    hidden,
    subagent_request_id,
    subagent_event,
    input_tokens,
    output_tokens,
    total_tokens,
    reasoning_tokens,
    cache_creation_tokens,
    cache_read_tokens,
    context_limit,
    compressed
) VALUES (
    @chat_id::uuid,
    @role::text,
    sqlc.narg('content')::jsonb,
    sqlc.narg('tool_call_id')::text,
    sqlc.narg('thinking')::text,
    @hidden::boolean,
    sqlc.narg('subagent_request_id')::uuid,
    sqlc.narg('subagent_event')::text,
    sqlc.narg('input_tokens')::bigint,
    sqlc.narg('output_tokens')::bigint,
    sqlc.narg('total_tokens')::bigint,
    sqlc.narg('reasoning_tokens')::bigint,
    sqlc.narg('cache_creation_tokens')::bigint,
    sqlc.narg('cache_read_tokens')::bigint,
    sqlc.narg('context_limit')::bigint,
    COALESCE(sqlc.narg('compressed')::boolean, FALSE)
)
RETURNING
    *;

-- name: GetLatestPendingSubagentRequestIDByChatID :one
WITH requests AS (
    SELECT
        subagent_request_id,
        MAX(created_at) AS requested_at
    FROM
        chat_messages
    WHERE
        chat_id = @chat_id::uuid
        AND subagent_request_id IS NOT NULL
        AND subagent_event = 'request'
    GROUP BY
        subagent_request_id
)
SELECT
    COALESCE(
        requests.subagent_request_id,
        '00000000-0000-0000-0000-000000000000'::uuid
    ) AS subagent_request_id
FROM
    requests
WHERE
    NOT EXISTS (
        SELECT
            1
        FROM
            chat_messages responses
        WHERE
            responses.chat_id = @chat_id::uuid
            AND responses.subagent_request_id = requests.subagent_request_id
            AND responses.subagent_event = 'response'
    )
ORDER BY
    requests.requested_at DESC
LIMIT
    1;

-- name: GetSubagentResponseMessageByChatIDAndRequestID :one
SELECT
    *
FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid
    AND subagent_request_id = @subagent_request_id::uuid
    AND subagent_event = 'response'
ORDER BY
    created_at DESC
LIMIT
    1;

-- name: GetSubagentRequestDurationByChatIDAndRequestID :one
WITH request AS (
    SELECT
        MIN(created_at) AS created_at
    FROM
        chat_messages
    WHERE
        chat_id = @chat_id::uuid
        AND subagent_request_id = @subagent_request_id::uuid
        AND subagent_event = 'request'
),
response AS (
    SELECT
        MAX(created_at) AS created_at
    FROM
        chat_messages
    WHERE
        chat_id = @chat_id::uuid
        AND subagent_request_id = @subagent_request_id::uuid
        AND subagent_event = 'response'
)
SELECT
    COALESCE(
        CAST(EXTRACT(EPOCH FROM (response.created_at - request.created_at)) * 1000 AS BIGINT),
        0::BIGINT
    )::BIGINT AS duration_ms
FROM
    request,
    response;

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

-- name: UpdateChatModelConfigByChatID :one
UPDATE
    chats
SET
    model_config = @model_config::jsonb,
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

-- name: InsertChatQueuedMessage :one
INSERT INTO chat_queued_messages (chat_id, content)
VALUES (@chat_id, @content)
RETURNING *;

-- name: GetChatQueuedMessages :many
SELECT * FROM chat_queued_messages
WHERE chat_id = @chat_id
ORDER BY id ASC;

-- name: DeleteChatQueuedMessage :exec
DELETE FROM chat_queued_messages WHERE id = @id AND chat_id = @chat_id;

-- name: DeleteAllChatQueuedMessages :exec
DELETE FROM chat_queued_messages WHERE chat_id = @chat_id;

-- name: PopNextQueuedMessage :one
DELETE FROM chat_queued_messages
WHERE id = (
    SELECT cqm.id FROM chat_queued_messages cqm
    WHERE cqm.chat_id = @chat_id
    ORDER BY cqm.id ASC
    LIMIT 1
)
RETURNING *;

-- name: GetChatByIDForUpdate :one
SELECT * FROM chats WHERE id = @id::uuid FOR UPDATE;
