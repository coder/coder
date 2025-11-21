-- name: GetProvisionerLogsAfterID :many
SELECT
	*
FROM
	provisioner_job_logs
WHERE
	job_id = @job_id
	AND (
		id > @created_after
	) ORDER BY id ASC;

-- name: InsertProvisionerJobLogs :many
INSERT INTO
	provisioner_job_logs
SELECT
	@job_id :: uuid AS job_id,
	unnest(@created_at :: timestamptz [ ]) AS created_at,
	unnest(@source :: log_source [ ]) AS source,
	unnest(@level :: log_level [ ]) AS LEVEL,
	unnest(@stage :: VARCHAR(128) [ ]) AS stage,
	unnest(@output :: VARCHAR(1024) [ ]) AS output RETURNING *;

-- name: UpdateProvisionerJobLogsOverflowed :exec
UPDATE
	provisioner_jobs
SET
	logs_overflowed = $2
WHERE
	id = $1;

-- name: UpdateProvisionerJobLogsLength :exec
UPDATE
	provisioner_jobs
SET
	logs_length = logs_length + $2
WHERE
	id = $1;
