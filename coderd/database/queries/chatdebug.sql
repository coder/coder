-- name: InsertChatDebugRun :one
INSERT INTO chat_debug_runs (
    chat_id,
    root_chat_id,
    parent_chat_id,
    model_config_id,
    trigger_message_id,
    history_tip_message_id,
    kind,
    status,
    provider,
    model,
    summary,
    started_at,
    updated_at,
    finished_at
)
VALUES (
    @chat_id::uuid,
    sqlc.narg('root_chat_id')::uuid,
    sqlc.narg('parent_chat_id')::uuid,
    sqlc.narg('model_config_id')::uuid,
    sqlc.narg('trigger_message_id')::bigint,
    sqlc.narg('history_tip_message_id')::bigint,
    @kind::text,
    @status::text,
    sqlc.narg('provider')::text,
    sqlc.narg('model')::text,
    COALESCE(sqlc.narg('summary')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('started_at')::timestamptz, NOW()),
    COALESCE(sqlc.narg('updated_at')::timestamptz, NOW()),
    sqlc.narg('finished_at')::timestamptz
)
RETURNING *;

-- name: UpdateChatDebugRun :one
-- Uses COALESCE so that passing NULL from Go means "keep the
-- existing value."  This is intentional: debug rows follow a
-- write-once-finalize pattern where fields are set at creation
-- or finalization and never cleared back to NULL.
UPDATE chat_debug_runs
SET
    root_chat_id = COALESCE(sqlc.narg('root_chat_id')::uuid, root_chat_id),
    parent_chat_id = COALESCE(sqlc.narg('parent_chat_id')::uuid, parent_chat_id),
    model_config_id = COALESCE(sqlc.narg('model_config_id')::uuid, model_config_id),
    trigger_message_id = COALESCE(sqlc.narg('trigger_message_id')::bigint, trigger_message_id),
    history_tip_message_id = COALESCE(sqlc.narg('history_tip_message_id')::bigint, history_tip_message_id),
    status = COALESCE(sqlc.narg('status')::text, status),
    provider = COALESCE(sqlc.narg('provider')::text, provider),
    model = COALESCE(sqlc.narg('model')::text, model),
    summary = COALESCE(sqlc.narg('summary')::jsonb, summary),
    finished_at = COALESCE(sqlc.narg('finished_at')::timestamptz, finished_at),
    updated_at = NOW()
WHERE id = @id::uuid
    AND chat_id = @chat_id::uuid
RETURNING *;

-- name: InsertChatDebugStep :one
INSERT INTO chat_debug_steps (
    run_id,
    chat_id,
    step_number,
    operation,
    status,
    history_tip_message_id,
    assistant_message_id,
    normalized_request,
    normalized_response,
    usage,
    attempts,
    error,
    metadata,
    started_at,
    updated_at,
    finished_at
)
SELECT
    @run_id::uuid,
    run.chat_id,
    @step_number::int,
    @operation::text,
    @status::text,
    sqlc.narg('history_tip_message_id')::bigint,
    sqlc.narg('assistant_message_id')::bigint,
    COALESCE(sqlc.narg('normalized_request')::jsonb, '{}'::jsonb),
    sqlc.narg('normalized_response')::jsonb,
    sqlc.narg('usage')::jsonb,
    COALESCE(sqlc.narg('attempts')::jsonb, '[]'::jsonb),
    sqlc.narg('error')::jsonb,
    COALESCE(sqlc.narg('metadata')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('started_at')::timestamptz, NOW()),
    COALESCE(sqlc.narg('updated_at')::timestamptz, NOW()),
    sqlc.narg('finished_at')::timestamptz
FROM chat_debug_runs run
WHERE run.id = @run_id::uuid
    AND run.chat_id = @chat_id::uuid
RETURNING *;

-- name: UpdateChatDebugStep :one
-- Uses COALESCE so that passing NULL from Go means "keep the
-- existing value."  This is intentional: debug rows follow a
-- write-once-finalize pattern where fields are set at creation
-- or finalization and never cleared back to NULL.
UPDATE chat_debug_steps
SET
    status = COALESCE(sqlc.narg('status')::text, status),
    history_tip_message_id = COALESCE(sqlc.narg('history_tip_message_id')::bigint, history_tip_message_id),
    assistant_message_id = COALESCE(sqlc.narg('assistant_message_id')::bigint, assistant_message_id),
    normalized_request = COALESCE(sqlc.narg('normalized_request')::jsonb, normalized_request),
    normalized_response = COALESCE(sqlc.narg('normalized_response')::jsonb, normalized_response),
    usage = COALESCE(sqlc.narg('usage')::jsonb, usage),
    attempts = COALESCE(sqlc.narg('attempts')::jsonb, attempts),
    error = COALESCE(sqlc.narg('error')::jsonb, error),
    metadata = COALESCE(sqlc.narg('metadata')::jsonb, metadata),
    finished_at = COALESCE(sqlc.narg('finished_at')::timestamptz, finished_at),
    updated_at = NOW()
WHERE id = @id::uuid
    AND chat_id = @chat_id::uuid
RETURNING *;

-- name: GetChatDebugRunsByChatID :many
-- Returns the most recent debug runs for a chat, ordered newest-first.
-- Callers must supply an explicit limit to avoid unbounded result sets.
SELECT *
FROM chat_debug_runs
WHERE chat_id = @chat_id::uuid
ORDER BY started_at DESC, id DESC
LIMIT @limit_val::int;

-- name: GetChatDebugRunByID :one
SELECT *
FROM chat_debug_runs
WHERE id = @id::uuid;

-- name: GetChatDebugStepsByRunID :many
SELECT *
FROM chat_debug_steps
WHERE run_id = @run_id::uuid
ORDER BY step_number ASC, started_at ASC;

-- name: DeleteChatDebugDataByChatID :execrows
DELETE FROM chat_debug_runs
WHERE chat_id = @chat_id::uuid;

-- name: DeleteChatDebugDataAfterMessageID :execrows
WITH affected_runs AS (
    SELECT DISTINCT run.id
    FROM chat_debug_runs run
    WHERE run.chat_id = @chat_id::uuid
        AND (
            run.history_tip_message_id > @message_id::bigint
            OR run.trigger_message_id > @message_id::bigint
        )

    UNION

    SELECT DISTINCT step.run_id AS id
    FROM chat_debug_steps step
    WHERE step.chat_id = @chat_id::uuid
        AND (
            step.assistant_message_id > @message_id::bigint
            OR step.history_tip_message_id > @message_id::bigint
        )
)
DELETE FROM chat_debug_runs
WHERE chat_id = @chat_id::uuid
    AND id IN (SELECT id FROM affected_runs);

-- name: FinalizeStaleChatDebugRows :one
-- Marks orphaned in-progress rows as interrupted so they do not stay
-- in a non-terminal state forever.  The NOT IN list must match the
-- terminal statuses defined by ChatDebugStatus in codersdk/chats.go.
--
-- The steps CTE also catches steps whose parent run was just finalized
-- (via run_id IN), because PostgreSQL data-modifying CTEs share the
-- same snapshot and cannot see each other's row updates.  Without this,
-- a step with a recent updated_at would survive its run's finalization
-- and remain in 'in_progress' state permanently.
WITH finalized_runs AS (
    UPDATE chat_debug_runs
    SET
        status = 'interrupted',
        updated_at = NOW(),
        finished_at = NOW()
    WHERE updated_at < @updated_before::timestamptz
        AND finished_at IS NULL
        AND status NOT IN ('completed', 'error', 'interrupted')
    RETURNING id
), finalized_steps AS (
    UPDATE chat_debug_steps
    SET
        status = 'interrupted',
        updated_at = NOW(),
        finished_at = NOW()
    WHERE (
            updated_at < @updated_before::timestamptz
            OR run_id IN (SELECT id FROM finalized_runs)
        )
        AND finished_at IS NULL
        AND status NOT IN ('completed', 'error', 'interrupted')
    RETURNING 1
)
SELECT
    (SELECT COUNT(*) FROM finalized_runs)::bigint AS runs_finalized,
    (SELECT COUNT(*) FROM finalized_steps)::bigint AS steps_finalized;
