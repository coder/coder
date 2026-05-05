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

-- name: ClearStaleChatRunnerReady :many
-- Clears chat_runner_ready on workspace agents that have been disconnected
-- past the agent stale threshold and still have at least one pending chat
-- bound to them. Coderd's AcquireChats query skips chats whose bound agent
-- still reports ready, so leaving the flag true after disconnect would
-- strand those chats indefinitely. Returns the IDs of the cleared agents
-- for observability.
UPDATE
    workspace_agents
SET
    chat_runner_ready = false,
    chat_runner_ready_at = NULL
WHERE
    chat_runner_ready = true
    AND disconnected_at IS NOT NULL
    AND disconnected_at < @stale_threshold::timestamptz
    AND EXISTS (
        SELECT 1
        FROM chats c
        WHERE c.agent_id = workspace_agents.id
            AND c.status = 'pending'::chat_status
            AND c.archived = false
    )
RETURNING id;
