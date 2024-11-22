-- Acquires the lock for a single job that isn't started, completed,
-- canceled, and that matches an array of provisioner types.
--
-- SKIP LOCKED is used to jump over locked rows. This prevents
-- multiple provisioners from acquiring the same jobs. See:
-- https://www.postgresql.org/docs/9.5/sql-select.html#SQL-FOR-UPDATE-SHARE
-- name: AcquireProvisionerJob :one
UPDATE
	provisioner_jobs
SET
	started_at = @started_at,
	updated_at = @started_at,
	worker_id = @worker_id
WHERE
	id = (
		SELECT
			id
		FROM
			provisioner_jobs AS potential_job
		WHERE
			potential_job.started_at IS NULL
			AND potential_job.organization_id = @organization_id
			-- Ensure the caller has the correct provisioner.
			AND potential_job.provisioner = ANY(@types :: provisioner_type [ ])
			-- elsewhere, we use the tagset type, but here we use jsonb for backward compatibility
			-- they are aliases and the code that calls this query already relies on a different type
			AND provisioner_tagset_contains(@provisioner_tags :: jsonb, potential_job.tags :: jsonb)
		ORDER BY
			potential_job.created_at
		FOR UPDATE
		SKIP LOCKED
		LIMIT
			1
	) RETURNING *;

-- name: GetProvisionerJobByID :one
SELECT
	*
FROM
	provisioner_jobs
WHERE
	id = $1;

-- name: GetProvisionerJobsByIDs :many
SELECT
	*
FROM
	provisioner_jobs
WHERE
	id = ANY(@ids :: uuid [ ]);

-- name: GetProvisionerJobsByIDsWithQueuePosition :many
WITH unstarted_jobs AS (
    SELECT
        id, created_at
    FROM
        provisioner_jobs
    WHERE
        started_at IS NULL
),
queue_position AS (
    SELECT
        id,
        ROW_NUMBER() OVER (ORDER BY created_at ASC) AS queue_position
    FROM
        unstarted_jobs
),
queue_size AS (
	SELECT COUNT(*) as count FROM unstarted_jobs
)
SELECT
	sqlc.embed(pj),
    COALESCE(qp.queue_position, 0) AS queue_position,
    COALESCE(qs.count, 0) AS queue_size
FROM
	provisioner_jobs pj
LEFT JOIN
	queue_position qp ON qp.id = pj.id
LEFT JOIN
	queue_size qs ON TRUE
WHERE
	pj.id = ANY(@ids :: uuid [ ]);

-- name: GetProvisionerJobsCreatedAfter :many
SELECT * FROM provisioner_jobs WHERE created_at > $1;

-- name: InsertProvisionerJob :one
INSERT INTO
	provisioner_jobs (
		id,
		created_at,
		updated_at,
		organization_id,
		initiator_id,
		provisioner,
		storage_method,
		file_id,
		"type",
		"input",
		tags,
		trace_metadata
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: UpdateProvisionerJobByID :exec
UPDATE
	provisioner_jobs
SET
	updated_at = $2
WHERE
	id = $1;

-- name: UpdateProvisionerJobWithCancelByID :exec
UPDATE
	provisioner_jobs
SET
	canceled_at = $2,
	completed_at = $3
WHERE
	id = $1;

-- name: UpdateProvisionerJobWithCompleteByID :exec
UPDATE
	provisioner_jobs
SET
	updated_at = $2,
	completed_at = $3,
	error = $4,
	error_code = $5
WHERE
	id = $1;

-- name: GetHungProvisionerJobs :many
SELECT
	*
FROM
	provisioner_jobs
WHERE
	updated_at < $1
	AND started_at IS NOT NULL
	AND completed_at IS NULL;

-- name: InsertProvisionerJobTimings :many
INSERT INTO provisioner_job_timings (job_id, started_at, ended_at, stage, source, action, resource)
SELECT
    @job_id::uuid AS provisioner_job_id,
    unnest(@started_at::timestamptz[]),
    unnest(@ended_at::timestamptz[]),
    unnest(@stage::provisioner_job_timing_stage[]),
    unnest(@source::text[]),
    unnest(@action::text[]),
    unnest(@resource::text[])
RETURNING *;

-- name: GetProvisionerJobTimingsByJobID :many
SELECT * FROM provisioner_job_timings
WHERE job_id = $1
ORDER BY started_at ASC;
