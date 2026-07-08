-- name: InsertChatToolCallExecution :one
-- Reserves an execution record for a tool call. The caller detects a
-- unique violation to learn whether it created the row or a previous
-- attempt already reserved it.
INSERT INTO chat_tool_call_executions (
    chat_id,
    tool_call_id,
    command,
    background,
    timeout_secs,
    created_at
) VALUES (
    @chat_id::uuid,
    @tool_call_id::text,
    @command::text,
    @background::boolean,
    @timeout_secs::bigint,
    @created_at::timestamptz
) RETURNING *;

-- name: GetChatToolCallExecution :one
SELECT * FROM chat_tool_call_executions
WHERE chat_id = @chat_id::uuid
  AND tool_call_id = @tool_call_id::text;

-- name: GetChatToolCallExecutions :many
SELECT * FROM chat_tool_call_executions
WHERE chat_id = @chat_id::uuid
  AND tool_call_id = ANY(@tool_call_ids::text[]);

-- name: UpdateChatToolCallExecutionProcess :one
-- Records the process handle once the process has started. started_at
-- is set together with process_id.
UPDATE chat_tool_call_executions
SET process_id = @process_id::text,
    workspace_agent_id = @workspace_agent_id::uuid,
    started_at = @started_at::timestamptz
WHERE chat_id = @chat_id::uuid
  AND tool_call_id = @tool_call_id::text
RETURNING *;

-- name: DeleteChatToolCallExecutions :exec
DELETE FROM chat_tool_call_executions
WHERE chat_id = @chat_id::uuid
  AND tool_call_id = ANY(@tool_call_ids::text[]);

-- name: DeleteOldChatToolCallExecutions :execrows
-- Deletes stale execution records. Rows left behind by attempts that
-- never cleaned up are harmless because resolved tool calls are never
-- re-executed; this is the janitor for those leftovers.
DELETE FROM chat_tool_call_executions
WHERE created_at < @before_time::timestamptz;
