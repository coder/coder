-- name: UpdateWorkspaceAgentChatRunnerStatus :exec
UPDATE
    workspace_agents
SET
    chat_runner_ready = @chat_runner_ready::boolean,
    chat_runner_ready_at = CASE
        WHEN @chat_runner_ready::boolean THEN NOW()
        ELSE NULL
    END
WHERE
    id = @agent_id::uuid;

-- name: GetPendingChatsForAgent :many
SELECT
    id,
    title,
    created_at
FROM
    chats
WHERE
    agent_id = @agent_id::uuid
    AND status = 'pending'::chat_status
ORDER BY
    updated_at ASC
LIMIT
    @max_chats::int;

-- name: AcquireChatForAgent :one
UPDATE
    chats
SET
    status = 'running'::chat_status,
    started_at = @started_at::timestamptz,
    heartbeat_at = @started_at::timestamptz,
    updated_at = @started_at::timestamptz,
    worker_id = @agent_id::uuid,
    runner_type = 'workspace_agent'::chat_runner_type,
    lease_epoch = lease_epoch + 1
WHERE
    id = (
        SELECT
            id
        FROM
            chats
        WHERE
            id = @chat_id::uuid
            AND agent_id = @agent_id::uuid
            AND status = 'pending'::chat_status
        FOR UPDATE
            SKIP LOCKED
        LIMIT
            1
    )
RETURNING
    *;

-- name: RenewChatLeaseByAgent :one
UPDATE
    chats
SET
    heartbeat_at = @now::timestamptz,
    updated_at = @now::timestamptz
WHERE
    id = @chat_id::uuid
    AND worker_id = @agent_id::uuid
    AND status = 'running'::chat_status
    AND lease_epoch = @lease_epoch::bigint
RETURNING
    id;

-- name: GetWorkspaceAgentChatRunnerState :one
-- Returns the chat runner readiness and connection state for a workspace agent.
-- Used during stale chat recovery for observability logging.
SELECT
    chat_runner_ready,
    chat_runner_ready_at,
    last_connected_at,
    disconnected_at
FROM
    workspace_agents
WHERE
    id = @agent_id::uuid;
