-- name: GetProvisionerLogsByIDBetween :many
SELECT
	*
FROM
	provisioner_job_logs
WHERE
	job_id = @job_id
	AND (
		created_at >= @created_after
		OR created_at <= @created_before
	)
ORDER BY
	created_at DESC;

-- name: InsertProvisionerJobLogs :many
INSERT INTO
	provisioner_job_logs
SELECT
	unnest(@id :: uuid [ ]) AS id,
	@job_id :: uuid AS job_id,
	unnest(@created_at :: timestamptz [ ]) AS created_at,
	unnest(@source :: log_source [ ]) AS source,
	unnest(@level :: log_level [ ]) AS LEVEL,
	unnest(@stage :: VARCHAR(128) [ ]) AS stage,
	unnest(@output :: VARCHAR(1024) [ ]) AS output RETURNING *;
