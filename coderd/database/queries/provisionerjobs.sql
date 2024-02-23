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
			provisioner_jobs AS nested
		WHERE
			nested.started_at IS NULL
			-- Ensure the caller has the correct provisioner.
			AND nested.provisioner = ANY(@types :: provisioner_type [ ])
			AND CASE
				-- Special case for untagged provisioners: only match untagged jobs.
				WHEN nested.tags :: jsonb = '{"scope": "organization", "owner": ""}' :: jsonb
				THEN nested.tags :: jsonb = @tags :: jsonb
				-- Ensure the caller satisfies all job tags.
				ELSE nested.tags :: jsonb <@ @tags :: jsonb
			END
		ORDER BY
			nested.created_at
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
