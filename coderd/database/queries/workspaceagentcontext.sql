-- name: UpsertWorkspaceAgentContextSnapshot :one
INSERT INTO workspace_agent_context_snapshots (
    workspace_agent_id,
    version,
    aggregate_hash,
    snapshot_error,
    received_at
) VALUES (
    @workspace_agent_id,
    @version,
    @aggregate_hash,
    @snapshot_error,
    @received_at
)
ON CONFLICT (workspace_agent_id) DO UPDATE SET
    version = EXCLUDED.version,
    aggregate_hash = EXCLUDED.aggregate_hash,
    snapshot_error = EXCLUDED.snapshot_error,
    received_at = EXCLUDED.received_at
RETURNING *;

-- name: UpsertWorkspaceAgentContextResource :one
INSERT INTO workspace_agent_context_resources (
    workspace_agent_id,
    source,
    body_kind,
    body,
    content_hash,
    size_bytes,
    status,
    error,
    source_path,
    created_at,
    updated_at
) VALUES (
    @workspace_agent_id,
    @source,
    @body_kind,
    @body,
    @content_hash,
    @size_bytes,
    @status,
    @error,
    @source_path,
    @now,
    @now
)
ON CONFLICT (workspace_agent_id, source) DO UPDATE SET
    body_kind = EXCLUDED.body_kind,
    body = EXCLUDED.body,
    content_hash = EXCLUDED.content_hash,
    size_bytes = EXCLUDED.size_bytes,
    status = EXCLUDED.status,
    error = EXCLUDED.error,
    source_path = EXCLUDED.source_path,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: DeleteStaleWorkspaceAgentContextResources :exec
-- Deletes any resources for the agent whose source is not in the
-- supplied active set. Atomic alongside the snapshot upsert so the
-- stored snapshot and resource rows always agree.
DELETE FROM workspace_agent_context_resources
WHERE workspace_agent_id = @workspace_agent_id
  AND NOT (source = ANY(@active_sources :: text[]));

-- name: GetLatestWorkspaceAgentContextSnapshot :one
SELECT * FROM workspace_agent_context_snapshots
WHERE workspace_agent_id = @workspace_agent_id;

-- name: ListWorkspaceAgentContextResources :many
SELECT * FROM workspace_agent_context_resources
WHERE workspace_agent_id = @workspace_agent_id
ORDER BY source ASC;
