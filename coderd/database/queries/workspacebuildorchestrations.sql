-- name: InsertWorkspaceBuildOrchestration :one
INSERT INTO workspace_build_orchestrations (
    id,
    created_at,
    updated_at,
    parent_build_id,
    workspace_id,
    child_transition,
    child_template_version_id,
    child_template_version_preset_id,
    child_rich_parameter_values,
    child_log_level,
    child_reason,
    status,
    error
)
VALUES (
    @id,
    @created_at,
    @updated_at,
    @parent_build_id,
    (SELECT workspace_id FROM workspace_builds WHERE id = @parent_build_id),
    @child_transition,
    @child_template_version_id,
    @child_template_version_preset_id,
    @child_rich_parameter_values,
    @child_log_level,
    @child_reason,
    'pending',
    NULL
)
RETURNING *;

-- name: GetNextPendingWorkspaceBuildOrchestrationForUpdate :one
-- Must be called from within a transaction. The row lock is released
-- when the transaction ends.
SELECT
    wbo.*
FROM
    workspace_build_orchestrations wbo
    JOIN workspace_builds wb ON wbo.parent_build_id = wb.id
    JOIN provisioner_jobs pj ON wb.job_id = pj.id
WHERE
    wbo.status = 'pending'
    AND (
        wbo.next_retry_after IS NULL
        OR wbo.next_retry_after <= NOW()
    )
    -- Include all terminal parent states so pending orchestration
    -- rows are processed and resolved even when no child build should
    -- be created.
    AND pj.job_status IN ('succeeded', 'failed', 'canceled')
ORDER BY
    wbo.created_at ASC
LIMIT 1
FOR UPDATE OF wbo SKIP LOCKED;

-- name: UpdateWorkspaceBuildOrchestrationCompletedByID :one
UPDATE
    workspace_build_orchestrations
SET
    child_build_id = @child_build_id,
    status = 'completed',
    next_retry_after = NULL,
    error = NULL,
    updated_at = @updated_at
WHERE
    id = @id
    AND status = 'pending'
RETURNING *;

-- name: UpdateWorkspaceBuildOrchestrationFailedByID :one
UPDATE
    workspace_build_orchestrations
SET
    status = 'failed',
    next_retry_after = NULL,
    error = @error,
    updated_at = @updated_at
WHERE
    id = @id
    AND status = 'pending'
RETURNING *;

-- name: UpdateWorkspaceBuildOrchestrationRetryByID :one
UPDATE
    workspace_build_orchestrations
SET
    attempt_count = attempt_count + 1,
    next_retry_after = CASE
        WHEN attempt_count + 1 >= @max_attempt_count::int THEN NULL
        ELSE @next_retry_after::timestamptz
    END,
    status = CASE
        WHEN attempt_count + 1 >= @max_attempt_count::int THEN 'failed'
        ELSE status
    END,
    error = @error,
    updated_at = @updated_at
WHERE
    id = @id
    AND status = 'pending'
RETURNING *;

-- name: UpdateWorkspaceBuildOrchestrationCanceledByID :one
UPDATE
    workspace_build_orchestrations
SET
    status = 'canceled',
    next_retry_after = NULL,
    error = NULL,
    updated_at = @updated_at
WHERE
    id = @id
    AND status = 'pending'
RETURNING *;

-- name: DeleteOldWorkspaceBuildOrchestrations :exec
WITH deletable AS (
    SELECT
        id
    FROM
        workspace_build_orchestrations
    WHERE
        status IN ('completed', 'failed', 'canceled')
        AND updated_at < @before_time::timestamptz
    ORDER BY
        updated_at ASC
    LIMIT @limit_count::int
)
DELETE FROM workspace_build_orchestrations
USING deletable
WHERE workspace_build_orchestrations.id = deletable.id;
