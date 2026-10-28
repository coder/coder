-- name: InsertFoundation :one
INSERT INTO foundations (id, organization_id, name, provisioner_tags, prevent_destroy_with_workspaces, created_at, updated_at)
VALUES (@id, @organization_id, @name, @provisioner_tags, COALESCE(sqlc.narg('prevent_destroy_with_workspaces')::boolean, true), @created_at, @updated_at) RETURNING *;

-- name: GetFoundationByID :one
SELECT * FROM foundations WHERE id = @id;

-- name: GetFoundationByTemplateID :one
SELECT * FROM foundations WHERE template_id = @template_id AND NOT deleted;

-- name: GetFoundationsByOrganizationID :many
SELECT * FROM foundations WHERE organization_id = @organization_id AND NOT deleted ORDER BY lower(name), id;

-- name: UpdateFoundation :one
UPDATE foundations SET name = @name, provisioner_tags = @provisioner_tags,
    prevent_destroy_with_workspaces = COALESCE(sqlc.narg('prevent_destroy_with_workspaces')::boolean, prevent_destroy_with_workspaces),
    updated_at = @updated_at
WHERE id = @id AND NOT deleted RETURNING *;

-- name: BindFoundation :one
UPDATE foundations SET template_id = @template_id, updated_at = @updated_at
WHERE id = @id AND NOT deleted AND (template_id IS NULL OR template_id = @template_id) RETURNING *;

-- name: DeleteFoundation :one
UPDATE foundations SET deleted = true, template_id = NULL, updated_at = @updated_at WHERE id = @id RETURNING *;

-- name: LockFoundation :exec
-- Invalidate repeatable-read snapshots, including an unconfigured deployment.
UPDATE foundations SET updated_at = updated_at WHERE id = @id;

-- name: GetFoundationConsumerCounts :one
-- Include every live consumer, even when hidden or only historically pinned.
WITH consumers AS (
    SELECT w.id FROM workspaces w WHERE NOT w.deleted AND (
        w.template_id IN (SELECT f.template_id FROM foundations f WHERE f.id = @foundation_id)
        OR EXISTS (SELECT 1 FROM workspace_builds b JOIN template_resource_revisions r
            ON r.id IN (b.template_resource_revision_id, b.state_template_resource_revision_id)
            WHERE b.workspace_id = w.id AND r.foundation_id = @foundation_id)
    )
), classified AS (
    SELECT CASE
        WHEN latest.job_status = 'succeeded' AND latest.transition = 'start' THEN 'running'
        WHEN latest.job_status = 'succeeded' AND latest.transition = 'stop' THEN 'stopped'
        ELSE 'unknown'
    END AS status
    FROM consumers
    LEFT JOIN LATERAL (
        SELECT b.transition, j.job_status FROM workspace_builds b
        JOIN provisioner_jobs j ON j.id = b.job_id
        WHERE b.workspace_id = consumers.id
        ORDER BY b.build_number DESC LIMIT 1
    ) latest ON true
)
SELECT count(*) AS total,
    count(*) FILTER (WHERE status = 'running') AS running,
    count(*) FILTER (WHERE status = 'stopped') AS stopped,
    count(*) FILTER (WHERE status = 'unknown') AS unknown
FROM classified;

-- name: GetFoundationWorkspaceCount :one
-- Include pinned consumers even if their template's association changed.
SELECT count(*) FROM workspaces w WHERE NOT w.deleted AND (
    w.template_id IN (SELECT foundations.template_id FROM foundations WHERE foundations.id = @foundation_id)
    OR EXISTS (SELECT 1 FROM workspace_builds b JOIN template_resource_revisions r
        ON r.id IN (b.template_resource_revision_id, b.state_template_resource_revision_id)
            WHERE b.workspace_id = w.id AND r.foundation_id = @foundation_id)
);
