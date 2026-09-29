-- name: GetWorkspaceExecutionReceiptByID :one
SELECT * FROM workspace_execution_receipts WHERE id = @id;

-- name: GetWorkspaceExecutionReceiptByRequest :one
SELECT * FROM workspace_execution_receipts
WHERE session_id = @session_id AND actor_id = @actor_id AND request_id = @request_id;

-- name: InsertWorkspaceExecutionReceipt :one
INSERT INTO workspace_execution_receipts (
 id, session_id, organization_id, owner_id, actor_id, request_id, input_digest,
 workspace_id, workspace_owner_id, agent_id, agent_instance_id, process_id,
 admission_revision, created_at, updated_at, deadline, state
)
SELECT @id, s.id, s.organization_id, s.owner_id, @actor_id, @request_id, @input_digest,
 s.workspace_id, s.workspace_owner_id, @agent_id, @agent_instance_id, @process_id,
 s.revision, @created_at, @created_at, @deadline, 'dispatching'
FROM workspace_execution_sessions s
WHERE s.id = @session_id AND s.revision = @admission_revision
 AND s.workspace_id IS NOT NULL
 AND s.state IN ('active','retained') AND s.lease_expires_at > @created_at
RETURNING *;

-- name: UpdateWorkspaceExecutionReceipt :one
-- Observations may resolve uncertainty, but never replace an observed terminal result.
UPDATE workspace_execution_receipts
SET state = @state, exit_code = @exit_code, error = @error, updated_at = @updated_at
WHERE id = @id AND state NOT IN ('completed','not_started')
RETURNING *;

-- name: GetPendingWorkspaceExecutionReceipts :many
SELECT * FROM workspace_execution_receipts
WHERE session_id = @session_id AND state IN ('dispatching','running','unknown')
ORDER BY updated_at, id
LIMIT 100;

-- name: HasPendingWorkspaceExecutionReceipts :one
SELECT EXISTS (
 SELECT 1 FROM workspace_execution_receipts
 WHERE session_id = @session_id AND state IN ('dispatching','running','unknown')
);
