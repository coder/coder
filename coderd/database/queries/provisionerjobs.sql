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
			-- Ensure the caller satisfies all job tags.
			AND nested.tags <@ @tags :: jsonb
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
SELECT
	sqlc.embed(pj),
	COALESCE(a.queue_position, 0) AS queue_position,
	COALESCE(a.queue_size, 0) AS queue_size
FROM
	provisioner_jobs pj
LEFT JOIN (
	SELECT
		id,
		ROW_NUMBER() OVER (ORDER BY created_at) AS queue_position,
		COUNT(*) OVER (PARTITION BY started_at IS NULL) AS queue_size
	FROM
		provisioner_jobs
	WHERE
		started_at IS NULL
	GROUP BY id
) a ON a.id = pj.id
WHERE pj.id = ANY(@ids :: uuid [ ]);

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
