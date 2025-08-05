-- name: InsertLicense :one
INSERT INTO
	licenses (
	uploaded_at,
	jwt,
	exp,
	uuid
)
VALUES
	($1, $2, $3, $4) RETURNING *;

-- name: GetLicenses :many
SELECT *
FROM licenses
ORDER BY (id);

-- name: GetLicenseByID :one
SELECT
	*
FROM
	licenses
WHERE
	id = $1
LIMIT
	1;

-- name: GetUnexpiredLicenses :many
SELECT *
FROM licenses
WHERE exp > NOW()
ORDER BY (id);

-- name: DeleteLicense :one
DELETE
FROM licenses
WHERE id = $1
RETURNING id;

-- name: GetManagedAgentCount :one
-- This isn't strictly a license query, but it's related to license enforcement.
SELECT
	COUNT(DISTINCT wb.id) AS count
FROM
	workspace_builds AS wb
JOIN
	provisioner_jobs AS pj
ON
	wb.job_id = pj.id
WHERE
	wb.transition = 'start'::workspace_transition
	AND wb.has_ai_task = true
	-- Only count jobs that are pending, running or succeeded. Other statuses
	-- like cancel(ed|ing), failed or unknown are not considered as managed
	-- agent usage. These workspace builds are typically unusable anyway.
	AND pj.job_status IN (
		'pending'::provisioner_job_status,
		'running'::provisioner_job_status,
		'succeeded'::provisioner_job_status
	)
	-- Jobs are counted at the time they are created, not when they are
	-- completed, as pending jobs haven't completed yet.
	AND wb.created_at BETWEEN @start_time::timestamptz AND @end_time::timestamptz;
