-- name: InsertGitHubRunnerPool :one
INSERT INTO github_runner_pools (
    id, organization_id, owner_id, template_id, name, github_config_url,
    github_token, github_token_key_id, runner_group_id, max_runners,
    startup_timeout_seconds, job_timeout_seconds
) VALUES (
    @id, @organization_id, @owner_id, @template_id, @name, @github_config_url,
    @github_token, @github_token_key_id, @runner_group_id, @max_runners,
    @startup_timeout_seconds, @job_timeout_seconds
) RETURNING *;

-- name: GetGitHubRunnerPoolByID :one
SELECT * FROM github_runner_pools WHERE id = @id;

-- name: GetGitHubRunnerPools :many
SELECT * FROM github_runner_pools
WHERE (organization_id = @organization_id OR @organization_id::uuid = '00000000-0000-0000-0000-000000000000')
ORDER BY created_at, id;

-- name: UpdateGitHubRunnerPool :one
UPDATE github_runner_pools SET
    state = @state, max_runners = @max_runners, updated_at = CURRENT_TIMESTAMP
WHERE id = @id AND state <> 'deleted'
    AND (state <> 'draining' OR @state IN ('draining', 'deleted'))
RETURNING *;

-- name: UpdateGitHubRunnerPoolStatus :exec
UPDATE github_runner_pools SET
    scale_set_id = @scale_set_id, assigned_jobs = @assigned_jobs,
    last_error = @last_error, updated_at = CURRENT_TIMESTAMP
WHERE id = @id AND lease_owner = @lease_owner;

-- name: AcquireGitHubRunnerPoolLease :one
UPDATE github_runner_pools SET
    lease_owner = @lease_owner, lease_expires_at = CURRENT_TIMESTAMP + INTERVAL '3 minutes'
WHERE id = @id AND state <> 'deleted'
    AND (lease_owner = @lease_owner OR lease_expires_at IS NULL OR lease_expires_at < CURRENT_TIMESTAMP)
RETURNING id;

-- name: ReleaseGitHubRunnerPoolLease :exec
UPDATE github_runner_pools SET lease_owner = NULL, lease_expires_at = NULL
WHERE id = @id AND lease_owner = @lease_owner;

-- name: UpdateEncryptedGitHubRunnerPoolToken :one
UPDATE github_runner_pools SET github_token = @github_token, github_token_key_id = @github_token_key_id
WHERE id = @id RETURNING *;

-- name: InsertGitHubRunner :one
INSERT INTO github_runners (id, organization_id, pool_id, name)
VALUES (@id, @organization_id, @pool_id, @name) RETURNING *;

-- name: GetGitHubRunnersByPoolID :many
SELECT * FROM github_runners WHERE pool_id = @pool_id ORDER BY created_at, id;

-- name: GetGitHubRunnerByID :one
SELECT * FROM github_runners WHERE id = @id;

-- name: UpdateGitHubRunner :one
UPDATE github_runners SET
    workspace_id = @workspace_id, github_runner_id = @github_runner_id,
    state = @state, job_request_id = @job_request_id, job_result = @job_result,
    last_error = @last_error, updated_at = CURRENT_TIMESTAMP
WHERE id = @id RETURNING *;

-- name: CancelGitHubRunnerWorkspaceBuild :exec
UPDATE provisioner_jobs AS j SET
    canceled_at = COALESCE(j.canceled_at, CURRENT_TIMESTAMP),
    completed_at = CASE WHEN j.worker_id IS NULL THEN CURRENT_TIMESTAMP ELSE j.completed_at END
WHERE j.id = @job_id AND j.completed_at IS NULL
    AND EXISTS (
        SELECT 1 FROM github_runners r
        JOIN workspace_builds b ON b.workspace_id = r.workspace_id
        WHERE r.id = @runner_id AND b.job_id = j.id
    );
