-- name: HasClosedWorkspaceExecutionAdmission :one
SELECT EXISTS (
    SELECT 1 FROM workspace_execution_sessions
    WHERE workspace_id = @workspace_id
      AND state NOT IN ('active', 'retained')
      AND (disposable OR state = 'preserving')
);

-- name: HasProtectedWorkspaceExecutionSession :one
-- Legacy deletion respects every adopted session as well as the disposal owner.
-- Stop and dormancy remain governed by their existing template policies.
SELECT EXISTS (
 SELECT 1 FROM workspace_execution_sessions s
 WHERE s.workspace_id = @workspace_id AND (
  s.retained OR s.lease_expires_at > @now::timestamptz
  OR (s.disposable AND s.state <> 'completed')
  OR s.state IN ('preserving','preservation_failed','deletion_failed')
  OR EXISTS (
   SELECT 1 FROM workspace_execution_receipts r
   WHERE r.workspace_id = s.workspace_id AND r.state IN ('dispatching','running','unknown')
  )
  OR EXISTS (
   SELECT 1 FROM chats c WHERE c.workspace_id = s.workspace_id
    AND (c.status IN ('running','interrupting','requires_action')
         OR EXISTS (SELECT 1 FROM chat_queued_messages q WHERE q.chat_id = c.id))
  )
  OR (COALESCE(NULLIF(s.declarations->'result_paths','null'::jsonb),'[]'::jsonb) <> '[]'::jsonb AND (
   s.state NOT IN ('preserved','completed')
   OR NOT EXISTS (
    SELECT 1 FROM workspace_execution_artifacts a
    WHERE a.session_id = s.id AND a.preservation_revision = s.revision
   )
   OR EXISTS (
    SELECT 1 FROM workspace_execution_artifacts a
    WHERE a.session_id = s.id AND a.preservation_revision = s.revision
     AND (a.expires_at <= @now::timestamptz
          OR octet_length(a.data) <> a.size_bytes OR sha256(a.data) <> a.sha256)
   )
  ))
 )
);

-- name: GetWorkspaceExecutionSessionByID :one
SELECT * FROM workspace_execution_sessions WHERE id = @id;

-- name: GetWorkspaceExecutionSessionByRequest :one
SELECT * FROM workspace_execution_sessions
WHERE organization_id = @organization_id AND actor_id = @actor_id AND request_id = @request_id;

-- name: InsertWorkspaceExecutionSession :one
INSERT INTO workspace_execution_sessions (
 id, organization_id, owner_id, actor_id, request_id, input_digest,
 workspace_id, workspace_owner_id, acquisition_build_id, created_at, updated_at, state,
 disposable, retained, lease_expires_at, declarations
) VALUES (
 @id, @organization_id, @owner_id, @actor_id, @request_id, @input_digest,
 @workspace_id, @workspace_owner_id, @acquisition_build_id, @created_at, @created_at, @state,
 @disposable, @retained, @lease_expires_at, @declarations
) RETURNING *;
