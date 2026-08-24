-- name: InsertChatExecutionStep :one
INSERT INTO chat_execution_steps (
    chat_id,
    history_version,
    generation_attempt,
    operation,
    outcome,
    runtime_ms
) VALUES (
    @chat_id::uuid,
    @history_version::bigint,
    @generation_attempt::bigint,
    @operation::chat_execution_step_operation,
    @outcome::chat_execution_step_outcome,
    @runtime_ms::bigint
)
RETURNING *;

-- name: GetChatExecutionStepByID :one
SELECT *
FROM chat_execution_steps
WHERE id = @id::uuid;

-- name: GetChatMessagesByExecutionStepID :many
-- Internal association inspection includes soft-deleted and non-user-visible
-- messages; its authorization wrapper requires a system read.
SELECT *
FROM chat_messages
WHERE execution_step_id = @execution_step_id::uuid
ORDER BY id ASC;

-- name: GetTotalChatExecutionRuntimeMsInRange :one
-- Computes hb_agent_runtime_v1 usage event payloads. Runtime remains billable
-- after message soft deletion or chat hard deletion.
SELECT COALESCE(SUM(runtime_ms), 0)::bigint AS total_runtime_ms
FROM chat_execution_steps
WHERE recorded_at >= @start_time::timestamptz
  AND recorded_at < @end_time::timestamptz;
