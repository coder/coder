-- name: UpdateTemplateResourceRunPlanPreview :exec
UPDATE template_resource_runs SET plan_preview = @plan_preview WHERE id = @id;

-- name: GetFoundationConsumerTemplateVersions :many
-- Scan active configuration and all versions used by non-deleted consumers.
-- Include historical pins, even when the Foundation has since been detached.
WITH consumers AS (
    SELECT w.id, w.template_id FROM workspaces w WHERE NOT w.deleted AND (
        w.template_id IN (SELECT f.template_id FROM foundations f WHERE f.id = @foundation_id)
        OR EXISTS (
            SELECT 1 FROM workspace_builds b
            JOIN template_resource_revisions r ON r.id = b.template_resource_revision_id
            WHERE b.workspace_id = w.id AND r.foundation_id = @foundation_id
        )
    )
), version_ids AS (
    SELECT t.active_version_id AS id FROM templates t
    WHERE NOT t.deleted AND (
        t.id IN (SELECT f.template_id FROM foundations f WHERE f.id = @foundation_id)
        OR t.id IN (SELECT consumers.template_id FROM consumers)
    )
    UNION
    SELECT b.template_version_id FROM workspace_builds b
    JOIN consumers c ON c.id = b.workspace_id
)
SELECT v.id, j.file_id FROM template_versions v
JOIN version_ids ON version_ids.id = v.id
JOIN provisioner_jobs j ON j.id = v.job_id
ORDER BY v.id;
