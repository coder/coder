-- name: InsertNetworkPolicy :one
INSERT INTO network_policies (id, organization_id, name, description, created_by, created_at, updated_at)
VALUES (@id, @organization_id, @name, @description, @created_by, @created_at, @updated_at)
RETURNING *;

-- name: GetNetworkPolicyByID :one
SELECT * FROM network_policies WHERE id = @id;

-- name: GetNetworkPolicyByIDForUpdate :one
SELECT * FROM network_policies WHERE id = @id FOR UPDATE;

-- name: GetNetworkPoliciesByOrganizationID :many
SELECT * FROM network_policies WHERE organization_id = @organization_id AND deleted_at IS NULL ORDER BY lower(name), id;

-- name: UpdateNetworkPolicy :one
UPDATE network_policies SET name = @name, description = @description, generation = generation + 1, updated_at = @updated_at
WHERE id = @id AND deletion_requested_at IS NULL AND deleted_at IS NULL RETURNING *;

-- name: PublishNetworkPolicyRevision :one
UPDATE network_policies SET published_revision_id = @published_revision_id, generation = generation + 1, updated_at = @updated_at
WHERE id = @id AND deletion_requested_at IS NULL AND deleted_at IS NULL RETURNING *;

-- name: InsertNetworkPolicyRevision :one
INSERT INTO network_policy_revisions (id, policy_id, organization_id, revision_number, hcl, file_id, created_by, created_at)
VALUES (@id, @policy_id, @organization_id, @revision_number, @hcl, @file_id, @created_by, @created_at)
RETURNING *;

-- name: GetNetworkPolicyRevisionByID :one
SELECT * FROM network_policy_revisions WHERE id = @id;

-- name: GetNetworkPolicyRevisionsByPolicyID :many
SELECT * FROM network_policy_revisions WHERE policy_id = @policy_id ORDER BY revision_number DESC;

-- name: InsertNetworkPolicyAttachment :one
INSERT INTO network_policy_attachments (policy_id, template_id, organization_id, created_at)
VALUES (@policy_id, @template_id, @organization_id, @created_at)
RETURNING *;

-- name: DeleteNetworkPolicyAttachment :execrows
DELETE FROM network_policy_attachments WHERE network_policy_attachments.policy_id = @policy_id AND network_policy_attachments.template_id = @template_id
AND NOT EXISTS (
    SELECT 1 FROM network_policy_targets WHERE network_policy_targets.policy_id = @policy_id
    AND active_job_id IS NOT NULL
);

-- name: GetNetworkPolicyAttachmentsByPolicyID :many
SELECT * FROM network_policy_attachments WHERE policy_id = @policy_id ORDER BY template_id;

-- name: InsertNetworkPolicyTarget :one
INSERT INTO network_policy_targets (id, policy_id, organization_id, cluster, context, namespace, tags, created_at, updated_at)
VALUES (@id, @policy_id, @organization_id, @cluster, @context, @namespace, @tags, @created_at, @updated_at)
RETURNING *;

-- name: GetNetworkPolicyTargetByID :one
SELECT * FROM network_policy_targets WHERE id = @id;

-- name: GetNetworkPolicyTargetByIDForUpdate :one
SELECT * FROM network_policy_targets WHERE id = @id FOR UPDATE;

-- name: GetNetworkPolicyTargetsByPolicyID :many
SELECT * FROM network_policy_targets WHERE policy_id = @policy_id ORDER BY cluster, context, namespace, id;

-- name: StartNetworkPolicyTargetJob :one
UPDATE network_policy_targets SET active_job_id = @job_id, last_job_id = @job_id,
    desired_revision_id = @desired_revision_id, generation = generation + 1,
    status = @status, last_error = '', updated_at = @updated_at,
    state_run_id = COALESCE(sqlc.narg('state_run_id')::uuid, state_run_id)
WHERE id = @id AND active_job_id IS NULL AND generation = @generation AND destroyed_at IS NULL
    AND sqlc.narg('job_id')::uuid IS NOT NULL AND sqlc.narg('desired_revision_id')::uuid IS NOT NULL
RETURNING *;

-- name: UpdateNetworkPolicyTargetState :one
UPDATE network_policy_targets SET state = @state, updated_at = @updated_at
WHERE id = @id AND active_job_id = @job_id AND generation = @generation
RETURNING *;

-- name: CompleteNetworkPolicyTargetJob :one
UPDATE network_policy_targets SET active_job_id = NULL, state = @state,
    applied_revision_id = @applied_revision_id, status = @status, drifted = @drifted,
    last_error = @last_error, updated_at = @updated_at,
    state_run_id = COALESCE(sqlc.narg('state_run_id')::uuid, state_run_id),
    destroyed_at = COALESCE(sqlc.narg('destroyed_at')::timestamptz, destroyed_at)
WHERE id = @id AND active_job_id = @job_id AND generation = @generation
RETURNING *;

-- name: InsertNetworkPolicyRun :one
INSERT INTO network_policy_runs (id, job_id, policy_id, target_id, revision_id, organization_id, generation, template_ids, operation, created_at)
SELECT @id, @job_id, @policy_id, @target_id, @revision_id, @organization_id, @generation,
    CASE WHEN @operation::text = 'destroy' THEN
        (SELECT template_ids FROM network_policy_runs WHERE network_policy_runs.id = network_policy_targets.state_run_id
            AND network_policy_runs.target_id = network_policy_targets.id AND network_policy_runs.revision_id = @revision_id)
    ELSE ARRAY(SELECT template_id FROM network_policy_attachments WHERE network_policy_attachments.policy_id = @policy_id ORDER BY template_id) END,
    @operation, @created_at
FROM network_policy_targets
WHERE network_policy_targets.id = @target_id
    AND network_policy_targets.policy_id = @policy_id
    AND network_policy_targets.organization_id = @organization_id
    AND network_policy_targets.active_job_id = @job_id
    AND network_policy_targets.desired_revision_id = @revision_id
    AND network_policy_targets.generation = @generation
RETURNING *;

-- name: GetNetworkPolicyRunByID :one
SELECT * FROM network_policy_runs WHERE id = @id;

-- name: GetNetworkPolicyRunByJobID :one
SELECT * FROM network_policy_runs WHERE job_id = @job_id;

-- name: GetNetworkPolicyRunsByPolicyID :many
SELECT * FROM network_policy_runs WHERE policy_id = @policy_id ORDER BY created_at DESC, id;

-- name: RequestNetworkPolicyDeletion :one
UPDATE network_policies SET deletion_requested_at = COALESCE(deletion_requested_at, @updated_at::timestamptz),
    generation = generation + 1, updated_at = @updated_at
WHERE id = @id AND deleted_at IS NULL RETURNING *;

-- name: CompleteNetworkPolicyDeletion :execrows
UPDATE network_policies SET deleted_at = @updated_at::timestamptz, updated_at = @updated_at,
    generation = generation + 1
WHERE network_policies.id = @id AND deletion_requested_at IS NOT NULL AND deleted_at IS NULL
    AND NOT EXISTS (SELECT 1 FROM network_policy_targets WHERE policy_id = @id AND destroyed_at IS NULL);

-- name: DestroyUnappliedNetworkPolicyTarget :one
UPDATE network_policy_targets SET destroyed_at = @updated_at::timestamptz, updated_at = @updated_at
WHERE network_policy_targets.id = @id AND network_policy_targets.generation = @generation AND active_job_id IS NULL
    AND destroyed_at IS NULL AND state = ''::bytea AND state_run_id IS NULL AND applied_revision_id IS NULL
    AND NOT EXISTS (
        SELECT 1 FROM network_policy_runs AS run JOIN provisioner_jobs AS job ON job.id = run.job_id
        WHERE run.target_id = network_policy_targets.id AND run.operation != 'plan'
            AND (job.completed_at IS NULL OR job.error_code IS DISTINCT FROM 'NETWORK_POLICY_STATE_UNCHANGED')
    )
RETURNING *;
