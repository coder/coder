-- name: InsertChatToolCallExecutionIntent :exec
-- Persists the execution intent for an execute tool call in the same
-- transaction that commits its assistant message. Conflicts on the
-- lineage key are no-ops so a replayed commit never resets a row.
INSERT INTO chat_tool_call_executions (
    id,
    chat_id,
    assistant_message_id,
    tool_call_id,
    input_sha256,
    status,
    created_at,
    updated_at
) VALUES (
    @id::uuid,
    @chat_id::uuid,
    @assistant_message_id::bigint,
    @tool_call_id::text,
    @input_sha256::text,
    'reserved',
    @created_at::timestamptz,
    @created_at::timestamptz
)
ON CONFLICT (chat_id, assistant_message_id, tool_call_id) DO NOTHING;

-- name: ClaimChatToolCallExecution :one
-- Claims an execution for dispatch. The insert arm covers calls whose
-- assistant message predates the ledger. The update arm is the
-- compare-and-set: only a reserved intent or a stale starting claim
-- can be taken over, and each takeover advances claim_epoch. The
-- input hash guard refuses to dispatch against a row created for
-- different input. Zero rows means the row exists but is not
-- claimable; the caller reads it to decide how to proceed.
INSERT INTO chat_tool_call_executions (
    id,
    chat_id,
    assistant_message_id,
    tool_call_id,
    input_sha256,
    status,
    command,
    background,
    timeout_secs,
    claim_epoch,
    claimed_at,
    created_at,
    updated_at
) VALUES (
    @id::uuid,
    @chat_id::uuid,
    @assistant_message_id::bigint,
    @tool_call_id::text,
    @input_sha256::text,
    'starting',
    @command::text,
    @background::boolean,
    @timeout_secs::bigint,
    1,
    @now::timestamptz,
    @now::timestamptz,
    @now::timestamptz
)
ON CONFLICT (chat_id, assistant_message_id, tool_call_id) DO UPDATE SET
    status = 'starting',
    command = EXCLUDED.command,
    background = EXCLUDED.background,
    timeout_secs = EXCLUDED.timeout_secs,
    claim_epoch = chat_tool_call_executions.claim_epoch + 1,
    claimed_at = EXCLUDED.claimed_at,
    updated_at = EXCLUDED.updated_at
WHERE chat_tool_call_executions.input_sha256 = EXCLUDED.input_sha256
  AND (chat_tool_call_executions.status = 'reserved'
   OR (chat_tool_call_executions.status = 'starting' AND chat_tool_call_executions.claimed_at < @stale_before::timestamptz))
RETURNING *;

-- name: GetChatToolCallExecution :one
SELECT * FROM chat_tool_call_executions
WHERE chat_id = @chat_id::uuid
  AND assistant_message_id = @assistant_message_id::bigint
  AND tool_call_id = @tool_call_id::text;

-- name: UpdateChatToolCallExecutionProcess :one
-- Records the started process on the claim that dispatched it. The
-- claim_epoch guard keeps a superseded claimer from overwriting the
-- process identity recorded by the current claim.
UPDATE chat_tool_call_executions
SET status = 'running',
    process_id = @process_id::text,
    workspace_agent_id = @workspace_agent_id::uuid,
    started_at = @started_at::timestamptz,
    updated_at = @started_at::timestamptz
WHERE chat_id = @chat_id::uuid
  AND assistant_message_id = @assistant_message_id::bigint
  AND tool_call_id = @tool_call_id::text
  AND claim_epoch = @claim_epoch::bigint
  AND status = 'starting'
RETURNING *;

-- name: UpdateChatToolCallExecutionStatus :one
-- Applies a lifecycle observation. from_statuses restricts which
-- lifecycle states may be overwritten; interrupt-owned states are
-- never listed, so tool observations cannot clobber them.
UPDATE chat_tool_call_executions
SET status = @status::chat_tool_call_execution_status,
    updated_at = @updated_at::timestamptz
WHERE chat_id = @chat_id::uuid
  AND assistant_message_id = @assistant_message_id::bigint
  AND tool_call_id = @tool_call_id::text
  AND status = ANY(@from_statuses::chat_tool_call_execution_status[])
RETURNING *;

-- name: MarkChatToolCallExecutionsResultCommitted :exec
-- Runs in the same transaction that commits the tool result messages
-- (real or synthetic). status is untouched so diagnostic states keep
-- their lifecycle truth after the chat has moved on.
UPDATE chat_tool_call_executions
SET result_committed_at = @result_committed_at::timestamptz,
    updated_at = @result_committed_at::timestamptz
WHERE chat_id = @chat_id::uuid
  AND assistant_message_id = @assistant_message_id::bigint
  AND tool_call_id = ANY(@tool_call_ids::text[])
  AND result_committed_at IS NULL;

-- name: MarkChatToolCallExecutionsInterrupted :many
-- Maps unresolved executions to their interrupt outcome in the same
-- transaction that commits the synthetic cancellation results:
-- background processes are deliberately left alive (detached),
-- never-dispatched reservations are canceled outright, and
-- dispatched foreground claims become cancel_requested for the
-- post-commit reconciler. Rows already in a terminal state keep it.
UPDATE chat_tool_call_executions
SET status = CASE
        WHEN background THEN 'detached'::chat_tool_call_execution_status
        WHEN status = 'reserved' THEN 'canceled'::chat_tool_call_execution_status
        ELSE 'cancel_requested'::chat_tool_call_execution_status
    END,
    updated_at = @updated_at::timestamptz
WHERE chat_id = @chat_id::uuid
  AND assistant_message_id = @assistant_message_id::bigint
  AND tool_call_id = ANY(@tool_call_ids::text[])
  AND status IN ('reserved', 'starting', 'running')
RETURNING *;

-- name: UpdateChatToolCallExecutionCancelOutcome :one
-- Resolves a cancel_requested row after a post-commit kill attempt:
-- canceled when termination was confirmed, unknown when the outcome
-- is unobservable, or cancel_requested to record a delivered signal
-- whose effect is unconfirmed.
UPDATE chat_tool_call_executions
SET status = @status::chat_tool_call_execution_status,
    cancel_signal_sent_at = COALESCE(sqlc.narg('cancel_signal_sent_at')::timestamptz, cancel_signal_sent_at),
    updated_at = @updated_at::timestamptz
WHERE chat_id = @chat_id::uuid
  AND assistant_message_id = @assistant_message_id::bigint
  AND tool_call_id = @tool_call_id::text
  AND status = 'cancel_requested'
RETURNING *;

-- name: DeleteOldChatToolCallExecutions :execrows
-- Age-based retention of ledger history, deleted in bounded batches
-- to keep transactions short. Deleting a row never affects a
-- still-running detached process, which stays addressable through
-- the process handle in its committed tool result; dedup protection
-- is not needed at this age because no attempt can still be
-- re-executing a call this old.
WITH deletable AS (
    SELECT id
    FROM chat_tool_call_executions
    WHERE created_at < @before_time::timestamptz
    ORDER BY created_at ASC
    LIMIT @limit_count::int
)
DELETE FROM chat_tool_call_executions
USING deletable
WHERE chat_tool_call_executions.id = deletable.id;
