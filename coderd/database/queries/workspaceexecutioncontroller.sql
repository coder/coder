-- name: UpdateWorkspaceExecutionSession :one
UPDATE workspace_execution_sessions SET
 state = @state, retained = @retained, lease_expires_at = @lease_expires_at,
 revision = @revision, delete_build_id = @delete_build_id,
 next_retry_at = @next_retry_at, attempt_count = @attempt_count,
 recovery_artifact_expires_at = sqlc.narg(recovery_artifact_expires_at)::timestamptz,
 error = @error, updated_at = @updated_at
WHERE id = @id AND revision = @expected_revision AND state = @expected_state
 AND @revision >= revision
RETURNING *;

-- name: GetReconciliableWorkspaceExecutionSessions :many
-- Independent cursors keep quiet maintenance from delaying actionable work.
-- New work wakes a parked session without requiring a session-row mutation.
WITH scheduling AS (
 SELECT s.id,
  EXISTS (
   SELECT 1 FROM workspace_execution_receipts r WHERE r.session_id = s.id
   AND r.state IN ('dispatching','running','unknown')
  ) OR EXISTS (
   SELECT 1 FROM workspaces w WHERE w.id = s.workspace_id AND w.deleted
  ) AS actionable,
  EXISTS (
   SELECT 1 FROM workspace_execution_receipts r WHERE r.session_id = s.id
   AND r.state IN ('dispatching','running','unknown')
   AND r.updated_at <= @observe_before::timestamptz
  ) OR (
   s.next_retry_at >= s.updated_at + interval '5 minutes'
   AND s.state NOT IN ('preserving','preservation_failed','deleting','deletion_failed')
   AND EXISTS (
    SELECT 1 FROM workspaces w WHERE w.id = s.workspace_id AND w.deleted
   )
  ) AS wake,
  s.retained OR (s.state = 'preserved' AND NOT s.disposable)
   OR s.lease_expires_at > @now::timestamptz
   OR (s.next_retry_at >= s.updated_at + interval '5 minutes'
       AND s.state NOT IN ('preserving','preservation_failed','deleting','deletion_failed')) AS parked
 FROM workspace_execution_sessions s
 WHERE s.id > @after_id::uuid AND s.workspace_id IS NOT NULL
 AND s.state <> 'completed'
)
SELECT s.* FROM workspace_execution_sessions s
JOIN scheduling q ON q.id = s.id
WHERE (q.wake OR s.next_retry_at IS NULL OR s.next_retry_at <= @now::timestamptz)
 AND @priority::boolean = (q.actionable OR NOT COALESCE(q.parked, false))
ORDER BY s.id
LIMIT 100;

-- name: LockWorkspaceExecutionWorkspace :exec
-- The row lock serializes ordinary owner transfers with task admission/delete.
SELECT id FROM workspaces WHERE id = @id FOR UPDATE;

-- name: HasBusyWorkspaceExecutionChats :one
-- Do not lock chat rows: chat admission takes its row lock before our lifecycle
-- lock. The shared lifecycle lock serializes admission against this snapshot.
SELECT EXISTS (
 SELECT 1 FROM chats c WHERE c.workspace_id = @workspace_id
 AND (
  c.status IN ('running','interrupting','requires_action')
  OR EXISTS (SELECT 1 FROM chat_queued_messages q WHERE q.chat_id = c.id)
 )
);

-- name: HasPendingWorkspaceExecutionReceiptsByWorkspaceID :one
SELECT EXISTS (
 SELECT 1 FROM workspace_execution_receipts
 WHERE workspace_id = sqlc.narg(workspace_id)
 AND state IN ('dispatching', 'running', 'unknown')
);

-- name: GetOtherWorkspaceExecutionSessionsByWorkspaceID :many
SELECT * FROM workspace_execution_sessions
WHERE workspace_id = sqlc.narg(workspace_id) AND id <> @id
ORDER BY id;
