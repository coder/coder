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
-- different input, and a row whose result already committed is never
-- claimable: its call is resolved in chat, so re-dispatching it
-- could run the command twice. The stale-takeover arm requires
-- stale_epoch to match the exact claim generation the caller
-- verified through the agent: evidence gathered against one claim
-- cannot take over a newer one, and a caller without a verified
-- epoch (NULL) can never take over a starting claim.
-- workspace_agent_id records the dispatch target before the
-- dispatch happens, so recovery can tell whether a token probe
-- reaches the agent the dead claimer actually targeted. Zero rows
-- means the row exists but is not claimable; the caller reads it
-- to decide how to proceed.
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
    workspace_agent_id,
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
    sqlc.narg('workspace_agent_id')::uuid,
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
    workspace_agent_id = EXCLUDED.workspace_agent_id,
    claim_epoch = chat_tool_call_executions.claim_epoch + 1,
    claimed_at = EXCLUDED.claimed_at,
    updated_at = EXCLUDED.updated_at
WHERE chat_tool_call_executions.input_sha256 = EXCLUDED.input_sha256
  AND chat_tool_call_executions.result_committed_at IS NULL
  AND (chat_tool_call_executions.status = 'reserved'
   OR (chat_tool_call_executions.status = 'starting'
       AND chat_tool_call_executions.claimed_at < @stale_before::timestamptz
       AND chat_tool_call_executions.claim_epoch = sqlc.narg('stale_epoch')::bigint))
RETURNING *;

-- name: GetChatToolCallExecution :one
SELECT * FROM chat_tool_call_executions
WHERE chat_id = @chat_id::uuid
  AND assistant_message_id = @assistant_message_id::bigint
  AND tool_call_id = @tool_call_id::text;

-- name: UpdateChatToolCallExecutionProcess :one
-- Records the started process on the claim that dispatched it. The
-- claim_epoch guard keeps a superseded claimer from overwriting the
-- process identity recorded by the current claim. An interrupt can
-- move the row out of starting (to cancel_requested or detached)
-- while the dispatch is still in flight; the handle write must
-- still land on those rows, without reverting the interrupt-owned
-- status, so the interrupt reconciler can kill the process instead
-- of resolving it unknown.
UPDATE chat_tool_call_executions
SET status = CASE WHEN status = 'starting' THEN 'running'::chat_tool_call_execution_status ELSE status END,
    process_id = @process_id::text,
    workspace_agent_id = @workspace_agent_id::uuid,
    started_at = @started_at::timestamptz,
    -- updated_at doubles as the sweep lease on cancel_requested
    -- rows: a late handle write anchored at an older start time
    -- must never move the lease backward and reopen a fresh
    -- interrupt to early sweep reclaim.
    updated_at = GREATEST(updated_at, @updated_at::timestamptz)
WHERE chat_id = @chat_id::uuid
  AND assistant_message_id = @assistant_message_id::bigint
  AND tool_call_id = @tool_call_id::text
  AND claim_epoch = @claim_epoch::bigint
  AND status IN ('starting', 'cancel_requested', 'detached')
RETURNING *;

-- name: UpdateChatToolCallExecutionStatus :one
-- Applies a lifecycle observation. from_statuses restricts which
-- lifecycle states may be overwritten; interrupt-owned states are
-- never listed, so tool observations cannot clobber them. A
-- non-null claim_epoch is an ownership guard mirroring
-- UpdateChatToolCallExecutionProcess: an observation from a
-- superseded claim matches no row instead of terminalizing the
-- claim that replaced it.
UPDATE chat_tool_call_executions
SET status = @status::chat_tool_call_execution_status,
    updated_at = @updated_at::timestamptz
WHERE chat_id = @chat_id::uuid
  AND assistant_message_id = @assistant_message_id::bigint
  AND tool_call_id = @tool_call_id::text
  AND status = ANY(@from_statuses::chat_tool_call_execution_status[])
  AND (sqlc.narg('claim_epoch')::bigint IS NULL OR claim_epoch = sqlc.narg('claim_epoch')::bigint)
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
-- Maps unresolved executions to their cancellation outcome in the
-- same transaction as the chat commit: never-dispatched reservations
-- are canceled outright, and dispatched claims without a resolved
-- handle (foreground, or background whose start is still in flight)
-- become cancel_requested for the post-commit reconciler. A
-- background row must not be terminalized as detached before its
-- handle lands: that would strand a running process with no
-- recoverable ID. Foreground detached rows (a timed-out wait) are
-- reopened to cancel_requested: only unresolved calls reach this
-- query, so their handle-bearing result never committed and the
-- process must be killed, not stranded behind a handle-less
-- synthetic cancellation.
-- spare_background selects the fate of a background process with a
-- recorded handle. Transitions that commit a synthetic result spare
-- it (detached) because the result carries the handle back to the
-- user. History-delete transitions must pass false: the deleted turn
-- commits no result, so a spared handle would have no carrier and
-- the process would leak; the row becomes cancel_requested and the
-- sweep kills it. Sparing also requires a live dispatch agent: a
-- NULLed or deleted workspace_agent_id means the process died with
-- its workspace and a spared handle would be unusable, so the row
-- routes to the cancel path and resolves terminally instead.
UPDATE chat_tool_call_executions
SET status = CASE
        WHEN status = 'reserved' THEN 'canceled'::chat_tool_call_execution_status
        WHEN @spare_background::boolean AND background AND process_id IS NOT NULL
             AND EXISTS (
                SELECT 1 FROM workspace_agents wa
                WHERE wa.id = chat_tool_call_executions.workspace_agent_id
                  AND NOT wa.deleted
             ) THEN 'detached'::chat_tool_call_execution_status
        ELSE 'cancel_requested'::chat_tool_call_execution_status
    END,
    updated_at = @updated_at::timestamptz
WHERE chat_id = @chat_id::uuid
  AND assistant_message_id = @assistant_message_id::bigint
  AND tool_call_id = ANY(@tool_call_ids::text[])
  AND status IN ('reserved', 'starting', 'running', 'detached')
RETURNING *;

-- name: MarkChatToolCallExecutionsCancelRequestedForHistoryDelete :many
-- A dispatched process is spared only while a committed tool result
-- carries its handle. History-delete transitions soft-delete every
-- message from the edited one onward, including the results that
-- carry the handles of process-bearing rows (background starts,
-- timed-out foregrounds, interrupt-spared backgrounds, and rows
-- whose best-effort detach write failed and stayed running), so
-- those processes would stay alive but unaddressable through the
-- chat. This routes every process-bearing row anchored at or after
-- the deleted boundary to cancel_requested for the sweep to kill,
-- matching the status coverage of the interrupt map for rows with
-- recorded process identity. starting claims are included: their
-- dispatch may have published a process whose handle write never
-- landed, and with the carrier deleted no retry will come back to
-- resolve the claim, so only the cancel path can reach the process.
-- Rows anchored before the boundary keep their carriers and are not
-- touched.
-- result_committed_at is re-stamped because the sweep anchors its
-- give-up bound on it: a long-detached row edited away later must
-- get its full kill-retry budget from the edit, not resolve unknown
-- with zero kill attempts because its original result committed
-- long ago.
-- from_message_id is the edited message's own ID (any role); the
-- comparison against assistant_message_id is correct because chat
-- message IDs are a single monotonic sequence across roles, so it
-- selects exactly the deleted-suffix turns.
UPDATE chat_tool_call_executions
SET status = 'cancel_requested'::chat_tool_call_execution_status,
    updated_at = @updated_at::timestamptz,
    result_committed_at = @updated_at::timestamptz
WHERE chat_id = @chat_id::uuid
  AND assistant_message_id >= @from_message_id::bigint
  AND status IN ('starting', 'running', 'detached')
RETURNING *;

-- name: ClaimStaleChatToolCallExecutionCancels :many
-- Claims a batch of cancel_requested rows whose reconciliation
-- stalled (the post-interrupt pass lost its agent dial or died).
-- Rows without recorded process identity are claimed too, so a
-- server crash between the interrupt commit and reconciliation
-- cannot strand them. Bumping updated_at inside the claim acts as
-- a cross-replica lease so concurrent sweepers do not hammer the
-- same unreachable agent; FOR UPDATE SKIP LOCKED keeps sweepers
-- from serializing on each other.
WITH candidates AS (
    SELECT id
    FROM chat_tool_call_executions
    WHERE status = 'cancel_requested'
      AND updated_at < @updated_before::timestamptz
    ORDER BY updated_at ASC
    LIMIT @limit_count::int
    FOR UPDATE SKIP LOCKED
)
UPDATE chat_tool_call_executions
SET updated_at = @now::timestamptz
FROM candidates
WHERE chat_tool_call_executions.id = candidates.id
RETURNING chat_tool_call_executions.*;

-- name: UpdateChatToolCallExecutionCancelOutcome :one
-- Resolves a cancel_requested row after a post-commit kill attempt:
-- canceled when termination was confirmed, unknown when the outcome
-- is unobservable, or cancel_requested to record a delivered signal
-- whose effect is unconfirmed. require_missing_process guards
-- outcomes decided from the absence of a process handle: a late
-- RecordStart can land identity on the row concurrently, and it
-- must win so the row stays cancel_requested and the sweep kills
-- the now-identified process.
UPDATE chat_tool_call_executions
SET status = @status::chat_tool_call_execution_status,
    cancel_signal_sent_at = COALESCE(sqlc.narg('cancel_signal_sent_at')::timestamptz, cancel_signal_sent_at),
    updated_at = @updated_at::timestamptz
WHERE chat_id = @chat_id::uuid
  AND assistant_message_id = @assistant_message_id::bigint
  AND tool_call_id = @tool_call_id::text
  AND status = 'cancel_requested'
  AND (NOT @require_missing_process::boolean OR process_id IS NULL)
RETURNING *;

-- name: DeleteOldChatToolCallExecutions :execrows
-- Age-based retention of ledger history, deleted in bounded batches
-- to keep transactions short. Only rows whose tool result was
-- committed are eligible: an uncommitted row still guards dedup for
-- a call a future retry may re-execute, no matter how old it is.
-- The window is anchored on result_committed_at, not created_at, so
-- a long-running execution that resolves late still gets the full
-- diagnostic retention after its result commits.
-- cancel_requested rows are kept regardless of age: the interrupt
-- commit stamps result_committed_at on them, but they still carry
-- the only stored identity of a process the sweep must kill.
-- running and detached rows are kept too: their processes may
-- still be alive, and a later history delete relies on these rows
-- to route the orphaned processes to the cancel path when the
-- committed results carrying their handles are deleted.
WITH deletable AS (
    SELECT id
    FROM chat_tool_call_executions
    WHERE result_committed_at < @before_time::timestamptz
      AND status NOT IN ('cancel_requested', 'running', 'detached')
    ORDER BY result_committed_at ASC
    LIMIT @limit_count::int
)
DELETE FROM chat_tool_call_executions
USING deletable
WHERE chat_tool_call_executions.id = deletable.id;

-- name: DeleteAbandonedChatToolCallExecutions :execrows
-- Long-horizon reaping of rows whose tool result never committed:
-- a crashed turn, a terminal task failure, or an unclaimed reserved
-- intent leaves result_committed_at NULL forever, and without this
-- delete such rows are only reaped by chat retention, which many
-- deployments leave unset. The horizon is anchored on updated_at so
-- any reconciler or re-attach activity defers deletion; a row idle
-- for the whole horizon guards a dedup that will never fire.
-- cancel_requested rows are excluded: the sweep owns them and
-- terminalizes them within the give-up bound. running and detached
-- rows are excluded like the committed arm above: they carry the
-- only stored identity of a process that may still be alive, and
-- deleting them would strand the history-delete cancel routing.
WITH deletable AS (
    SELECT id
    FROM chat_tool_call_executions
    WHERE updated_at < @before_time::timestamptz
      AND result_committed_at IS NULL
      AND status NOT IN ('cancel_requested', 'running', 'detached')
    ORDER BY updated_at ASC
    LIMIT @limit_count::int
)
DELETE FROM chat_tool_call_executions
USING deletable
WHERE chat_tool_call_executions.id = deletable.id;
