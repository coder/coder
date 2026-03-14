-- name: InsertChatRun :one
-- Creates a new chat run. The trigger auto-assigns the run number
-- by incrementing chats.last_run_number.
INSERT INTO chat_runs (
    chat_id
) VALUES (
    @chat_id::uuid
)
RETURNING
    *;

-- name: InsertChatRunStep :one
-- Creates a new chat run step. The trigger auto-assigns the step
-- number by incrementing chat_runs.last_step_number.
INSERT INTO chat_run_steps (
    chat_run_id,
    chat_id,
    model_config_id
) VALUES (
    @chat_run_id::uuid,
    @chat_id::uuid,
    sqlc.narg('model_config_id')::uuid
)
RETURNING
    *;

-- name: AcquireChatRunStep :one
-- Finds the oldest unclaimed active step and claims it for a worker
-- by setting worker_id on the parent chat_run. Uses SKIP LOCKED to
-- prevent multiple replicas from acquiring the same step.
UPDATE chat_runs
SET worker_id = @worker_id::uuid
WHERE id = (
    SELECT
        s.chat_run_id
    FROM
        chat_run_steps s
    JOIN
        chat_runs r ON r.id = s.chat_run_id
    WHERE
        s.completed_at IS NULL
        AND s.error IS NULL
        AND s.interrupted_at IS NULL
        AND r.worker_id IS NULL
    ORDER BY
        s.started_at ASC
    FOR UPDATE OF r
        SKIP LOCKED
    LIMIT
        1
)
RETURNING
    *;

-- name: UpdateChatRunStepHeartbeat :execrows
-- Bumps the heartbeat timestamp for an active step so that other
-- replicas know the worker is still alive. Verifies the step belongs
-- to a run owned by the given worker.
UPDATE chat_run_steps
SET heartbeat_at = NOW()
WHERE
    id = @id::uuid
    AND completed_at IS NULL
    AND error IS NULL
    AND interrupted_at IS NULL
    AND chat_run_id IN (
        SELECT id FROM chat_runs
        WHERE id = chat_run_steps.chat_run_id
          AND worker_id = @worker_id::uuid
    );

-- name: CompleteChatRunStep :one
-- Marks a step as completed with usage stats and tool call counts.
UPDATE chat_run_steps
SET
    completed_at = @completed_at::timestamptz,
    continuation_reason = sqlc.narg('continuation_reason')::text,
    input_tokens = sqlc.narg('input_tokens')::integer,
    output_tokens = sqlc.narg('output_tokens')::integer,
    total_tokens = sqlc.narg('total_tokens')::integer,
    reasoning_tokens = sqlc.narg('reasoning_tokens')::integer,
    cache_creation_tokens = sqlc.narg('cache_creation_tokens')::integer,
    cache_read_tokens = sqlc.narg('cache_read_tokens')::integer,
    context_limit = sqlc.narg('context_limit')::integer,
    tool_calls_total = @tool_calls_total::integer,
    tool_calls_completed = @tool_calls_completed::integer,
    tool_calls_errored = @tool_calls_errored::integer
WHERE
    id = @id::uuid
    AND completed_at IS NULL
    AND error IS NULL
    AND interrupted_at IS NULL
RETURNING
    *;

-- name: ErrorChatRunStep :one
-- Sets an error on a step. Errors are terminal, so completed_at is
-- also set.
UPDATE chat_run_steps
SET
    error = @error::text,
    completed_at = NOW()
WHERE
    id = @id::uuid
    AND completed_at IS NULL
    AND error IS NULL
    AND interrupted_at IS NULL
RETURNING
    *;

-- name: InterruptChatRunStep :one
-- Marks a step as interrupted by the user.
UPDATE chat_run_steps
SET
    interrupted_at = NOW()
WHERE
    id = @id::uuid
    AND completed_at IS NULL
    AND error IS NULL
    AND interrupted_at IS NULL
RETURNING
    *;

-- name: InterruptActiveChatRunStep :exec
-- Interrupts whatever active step exists for a chat. Used when the
-- user sends a new message with interrupt behavior.
UPDATE chat_run_steps
SET
    interrupted_at = NOW()
WHERE
    chat_id = @chat_id::uuid
    AND completed_at IS NULL
    AND error IS NULL
    AND interrupted_at IS NULL;

-- name: ErrorStalledChatRunSteps :exec
-- Finds active steps for a chat with stale heartbeats and marks them
-- as errored. Used by reconcileChatRun to clean up before creating
-- a new run.
UPDATE chat_run_steps
SET
    error = 'step stalled: heartbeat expired',
    completed_at = NOW()
WHERE
    chat_id = @chat_id::uuid
    AND heartbeat_at < @stale_threshold::timestamptz
    AND completed_at IS NULL
    AND error IS NULL
    AND interrupted_at IS NULL;

-- name: GetStaleChatRunSteps :many
-- Finds all active steps across all chats with stale heartbeats.
-- Used by recoverStaleChatRunSteps to detect and recover stuck work.
SELECT
    s.*,
    r.chat_id AS run_chat_id,
    r.worker_id AS run_worker_id
FROM
    chat_run_steps s
JOIN
    chat_runs r ON r.id = s.chat_run_id
WHERE
    s.completed_at IS NULL
    AND s.error IS NULL
    AND s.interrupted_at IS NULL
    AND r.worker_id IS NOT NULL
    AND s.heartbeat_at < @stale_threshold::timestamptz;

-- name: GetActiveChatRunStep :one
-- Gets the active step for a chat, if any. There can be at most one
-- thanks to the partial unique index.
SELECT
    *
FROM
    chat_run_steps
WHERE
    chat_id = @chat_id::uuid
    AND completed_at IS NULL
    AND error IS NULL
    AND interrupted_at IS NULL;

-- name: GetChatRunByID :one
SELECT
    *
FROM
    chat_runs
WHERE
    id = @id::uuid;

-- name: ClearChatRunWorker :exec
-- Clears worker_id on a run so it can be re-acquired by another
-- replica. Used during graceful shutdown.
UPDATE chat_runs
SET worker_id = NULL
WHERE
    id = @id::uuid;

