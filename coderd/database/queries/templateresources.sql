-- name: LockTemplateResourceTemplate :exec
-- A no-op update also invalidates stale repeatable-read snapshots. Merely
-- locking the row could miss a concurrently inserted first resource revision.
UPDATE templates SET updated_at = updated_at WHERE id = @id;

-- name: InsertTemplateResourceState :one
INSERT INTO template_resource_states (template_id, organization_id, updated_at)
VALUES (@template_id, @organization_id, @updated_at)
ON CONFLICT (template_id) DO UPDATE SET template_id = EXCLUDED.template_id
RETURNING *;

-- name: GetTemplateResourceState :one
SELECT * FROM template_resource_states WHERE template_id = @template_id;

-- name: GetTemplateResourceStateForUpdate :one
SELECT * FROM template_resource_states WHERE template_id = @template_id FOR UPDATE;

-- name: InsertTemplateResourceRevision :one
INSERT INTO template_resource_revisions (id, template_id, organization_id, file_id, source, created_by, created_at)
VALUES (@id, @template_id, @organization_id, @file_id, @source, @created_by, @created_at)
RETURNING *;

-- name: GetTemplateResourceRevisionByID :one
SELECT * FROM template_resource_revisions WHERE id = @id;

-- name: GetTemplateResourceRevisionsByTemplateID :many
SELECT * FROM template_resource_revisions WHERE template_id = @template_id ORDER BY created_at DESC, id;

-- name: SetTemplateResourceCurrentRevision :one
UPDATE template_resource_states SET current_revision_id = @current_revision_id, updated_at = @updated_at
WHERE template_id = @template_id RETURNING *;

-- name: InsertTemplateResourceRun :one
INSERT INTO template_resource_runs (id, template_id, organization_id, revision_id, job_id, action, created_at)
VALUES (@id, @template_id, @organization_id, @revision_id, @job_id, @action, @created_at)
RETURNING *;

-- name: GetTemplateResourceRunByID :one
SELECT * FROM template_resource_runs WHERE id = @id;

-- name: GetTemplateResourceRunByJobID :one
SELECT * FROM template_resource_runs WHERE job_id = @job_id;

-- name: GetTemplateResourceRunsByTemplateID :many
SELECT * FROM template_resource_runs WHERE template_id = @template_id ORDER BY created_at DESC, id;

-- name: StartTemplateResourceRun :one
UPDATE template_resource_states SET active_run_id = @run_id, updated_at = @updated_at
WHERE template_id = @template_id AND active_run_id IS NULL AND status != 'unknown'
    AND sqlc.narg('run_id')::uuid IS NOT NULL
RETURNING *;

-- name: UpdateTemplateResourceState :one
UPDATE template_resource_states SET state = @state, status = @status, updated_at = @updated_at
WHERE template_id = @template_id AND active_run_id = @run_id
RETURNING *;

-- name: CompleteTemplateResourceRun :one
UPDATE template_resource_states SET active_run_id = NULL, status = @status, state = @state,
    outputs = @outputs, applied_revision_id = @applied_revision_id, updated_at = @updated_at
WHERE template_id = @template_id AND active_run_id = @run_id
RETURNING *;

-- name: PinWorkspaceBuildTemplateResources :exec
UPDATE workspace_builds SET template_resource_revision_id = @template_resource_revision_id,
    template_resource_outputs = @template_resource_outputs
WHERE id = @id;
