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
-- existing value." This is intentional: debug rows follow a
-- write-once-finalize pattern where fields are set at creation
-- or finalization and never cleared back to NULL. The @now
-- parameter keeps updated_at under the caller's clock.
--
-- finished_at is enforced as write-once at the SQL level: once
-- populated it cannot be overwritten by a later call. Callers
-- that issue a summary or status refresh after the run has
-- already finalized therefore cannot corrupt the original
-- completion timestamp, which keeps duration and ordering
-- calculations stable regardless of how many times the row is
-- updated.
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
    finished_at = COALESCE(finished_at, sqlc.narg('finished_at')::timestamptz),
    updated_at = @now::timestamptz
WHERE id = @id::uuid
    AND chat_id = @chat_id::uuid
RETURNING *;

-- name: InsertChatDebugStep :one
-- The CTE atomically locks the parent run via UPDATE, bumps its
-- updated_at (eliminating a separate TouchChatDebugRunUpdatedAt
-- call), and enforces the finalization guard: if the run is already
-- finished, the UPDATE returns zero rows, the INSERT gets no source
-- rows, and sql.ErrNoRows is returned. The UPDATE also serializes
-- with concurrent FinalizeStale under READ COMMITTED isolation.
WITH locked_run AS (
    UPDATE chat_debug_runs
    SET updated_at = COALESCE(sqlc.narg('updated_at')::timestamptz, NOW())
    WHERE id = @run_id::uuid
        AND chat_id = @chat_id::uuid
        AND finished_at IS NULL
    RETURNING chat_id
)
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
    locked_run.chat_id,
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
FROM locked_run
RETURNING *;

-- name: UpdateChatDebugStep :one
-- Uses COALESCE so that passing NULL from Go means "keep the
-- existing value." This is intentional: debug rows follow a
-- write-once-finalize pattern where fields are set at creation
-- or finalization and never cleared back to NULL. The @now
-- parameter keeps updated_at under the caller's clock, matching
-- the injectable quartz.Clock used by FinalizeStale sweeps.
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
    updated_at = @now::timestamptz
WHERE id = @id::uuid
    AND chat_id = @chat_id::uuid
RETURNING *;

-- name: TouchChatDebugRunUpdatedAt :exec
-- Overrides updated_at on the parent run without touching any
-- other column. Used by tests that need to stamp a run with a
-- specific timestamp after the InsertChatDebugStep CTE has
-- already bumped it to NOW(), so stale-row finalization paths
-- can be exercised deterministically. The chatdebug service
-- itself does not call this: heartbeats go through
-- TouchChatDebugStepAndRun, and step creation updates the parent
-- run via the InsertChatDebugStep CTE.
UPDATE chat_debug_runs
SET updated_at = @now::timestamptz
WHERE id = @id::uuid
    AND chat_id = @chat_id::uuid;

-- name: TouchChatDebugStepAndRun :exec
-- Atomically bumps updated_at on both the step and its parent run
-- in a single statement. This prevents FinalizeStale from
-- interleaving between the two touches and finalizing a run whose
-- step heartbeat was just written.
--
-- The step UPDATE joins through touched_run (via FROM) and reads
-- its RETURNING rows. Per the PostgreSQL WITH semantics, RETURNING
-- is the only way to communicate values between a data-modifying
-- CTE and the main query, and consuming those rows forces the run
-- UPDATE to complete before the step UPDATE. That matches the
-- lock order used by FinalizeStaleChatDebugRows and avoids a
-- deadlock between concurrent heartbeats and stale sweeps. The
-- join also constrains the step update to the specified run so a
-- mismatched (run_id, step_id) pair cannot silently refresh an
-- unrelated step.
WITH touched_run AS (
    UPDATE chat_debug_runs
    SET updated_at = @now::timestamptz
    WHERE id = @run_id::uuid
        AND chat_id = @chat_id::uuid
    RETURNING id, chat_id
)
UPDATE chat_debug_steps
SET updated_at = @now::timestamptz
FROM touched_run
WHERE chat_debug_steps.id = @step_id::uuid
    AND chat_debug_steps.run_id = touched_run.id
    AND chat_debug_steps.chat_id = touched_run.chat_id;

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
-- The started_before bound prevents retried cleanup from deleting
-- runs created by a replacement turn that races ahead of the retry
-- window (for example, after an unarchive races with a pending
-- archive-cleanup retry).
DELETE FROM chat_debug_runs
WHERE chat_id = @chat_id::uuid
    AND started_at < @started_before::timestamptz;

-- name: DeleteChatDebugDataAfterMessageID :execrows
-- Deletes debug runs (and their cascaded steps) whose message IDs
-- exceed the cutoff. The started_before bound prevents retried
-- cleanup from deleting runs created by a replacement turn that
-- raced ahead of the retry window.
WITH affected_runs AS (
    SELECT DISTINCT run.id
    FROM chat_debug_runs run
    WHERE run.chat_id = @chat_id::uuid
        AND run.started_at < @started_before::timestamptz
        AND (
            run.history_tip_message_id > @message_id::bigint
            OR run.trigger_message_id > @message_id::bigint
        )

    UNION

    SELECT DISTINCT step.run_id AS id
    FROM chat_debug_steps step
    JOIN chat_debug_runs run ON run.id = step.run_id
        AND run.chat_id = step.chat_id
    WHERE step.chat_id = @chat_id::uuid
        AND run.started_at < @started_before::timestamptz
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
-- in a non-terminal state forever. The NOT IN list must match the
-- terminal statuses defined by ChatDebugStatus in codersdk/chats.go.
--
-- The steps CTE also catches steps whose parent run was just finalized
-- (via run_id IN), because PostgreSQL data-modifying CTEs share the
-- same snapshot and cannot see each other's row updates. Without this,
-- a step with a recent updated_at would survive its run's finalization
-- and remain in 'in_progress' state permanently.
--
-- @now is the caller's clock timestamp so that mock-clock tests stay
-- consistent with the @updated_before cutoff.
WITH finalized_runs AS (
    UPDATE chat_debug_runs
    SET
        status = 'interrupted',
        updated_at = @now::timestamptz,
        finished_at = @now::timestamptz
    WHERE updated_at < @updated_before::timestamptz
        AND finished_at IS NULL
        AND status NOT IN ('completed', 'error', 'interrupted')
    RETURNING id
), finalized_steps AS (
    UPDATE chat_debug_steps
    SET
        status = 'interrupted',
        updated_at = @now::timestamptz,
        finished_at = @now::timestamptz
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
