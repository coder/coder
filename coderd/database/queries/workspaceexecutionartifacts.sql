-- name: InsertWorkspaceExecutionArtifact :execrows
INSERT INTO workspace_execution_artifacts (
    id, organization_id, owner_id, session_id, preservation_revision,
    source_path, name, mimetype, size_bytes, sha256, data, created_at, expires_at
)
SELECT
    @id::uuid, s.organization_id, s.owner_id, s.id, @preservation_revision::bigint,
    @source_path::text, @name::text, @mimetype::text, @size_bytes::bigint,
    @sha256::bytea, @data::bytea, @created_at::timestamptz, sqlc.narg('expires_at')::timestamptz
FROM workspace_execution_sessions s
WHERE s.id = @session_id::uuid
    AND s.state = 'preserving'
    AND s.revision = @preservation_revision::bigint;

-- name: GetWorkspaceExecutionArtifactsBySessionID :many
SELECT id, organization_id, owner_id, session_id, preservation_revision,
    source_path, name, mimetype, size_bytes, sha256, created_at, expires_at
FROM workspace_execution_artifacts
WHERE session_id = @session_id::uuid
ORDER BY preservation_revision, source_path;

-- name: ReadWorkspaceExecutionArtifact :one
WITH bounds AS (
    SELECT @byte_offset::int AS byte_offset, @byte_limit::int AS byte_limit
)
SELECT id, organization_id, owner_id, session_id, preservation_revision,
    source_path, name, mimetype, size_bytes, sha256, created_at, expires_at,
    substr(data, bounds.byte_offset + 1, bounds.byte_limit) AS data
FROM workspace_execution_artifacts CROSS JOIN bounds
WHERE id = @id::uuid;
