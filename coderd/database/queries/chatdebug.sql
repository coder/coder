-- updated_at is the retention clock used by DeleteOldChatDebugRuns.
-- Set it on every write to keep retention semantics correct.
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
-- updated_at is also the retention clock used by DeleteOldChatDebugRuns.
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
-- updated_at, and enforces the finalization guard: if the run is already
-- finished, the UPDATE returns zero rows, the INSERT gets no source
-- rows, and sql.ErrNoRows is returned. The UPDATE also serializes
-- with a concurrent run finalization under READ COMMITTED isolation.
-- A zero @step_number numbers the step after the run's highest step.
-- The row lock does not make that number unique, because the statement
-- snapshot predates the lock wait. Callers hold LockIDChatDebugRunSteps
-- from an earlier statement of the same transaction, so the snapshot
-- includes every committed step.
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
    COALESCE(
        NULLIF(@step_number::int, 0),
        (SELECT COALESCE(MAX(s.step_number), 0) + 1 FROM chat_debug_steps s WHERE s.run_id = @run_id::uuid)
    ),
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
-- parameter keeps updated_at under the caller's clock.
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

-- updated_at is the retention clock, so the window starts after the run
-- stops being written to.
-- Intentionally no finished_at IS NOT NULL guard: abandoned in-flight rows
-- older than the cutoff are also purged.
-- name: DeleteOldChatDebugRuns :execrows
WITH deletable AS (
    SELECT id, chat_id
    FROM chat_debug_runs
    WHERE updated_at < @before_time::timestamptz
    ORDER BY updated_at ASC
    LIMIT @limit_count::int
)
DELETE FROM chat_debug_runs
USING deletable
WHERE chat_debug_runs.id = deletable.id
    AND chat_debug_runs.chat_id = deletable.chat_id;
