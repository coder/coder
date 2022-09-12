-- name: GetTemplateByID :one
SELECT
	*
FROM
	templates
WHERE
	id = $1
LIMIT
	1;

-- name: GetTemplatesWithFilter :many
SELECT
	*
FROM
	templates
WHERE
	-- Optionally include deleted templates
	templates.deleted = @deleted
	-- Filter by organization_id
	AND CASE
		WHEN @organization_id :: uuid != '00000000-00000000-00000000-00000000' THEN
			organization_id = @organization_id
		ELSE true
	END
	-- Filter by exact name
	AND CASE
		WHEN @exact_name :: text != '' THEN
			LOWER("name") = LOWER(@exact_name)
		ELSE true
	END
	-- Filter by ids
	AND CASE
		WHEN array_length(@ids :: uuid[], 1) > 0 THEN
			id = ANY(@ids)
		ELSE true
	END
ORDER BY (name, id) ASC
;

-- name: GetTemplateByOrganizationAndName :one
SELECT
	*
FROM
	templates
WHERE
	organization_id = @organization_id
	AND deleted = @deleted
	AND LOWER("name") = LOWER(@name)
LIMIT
	1;

-- name: GetTemplates :many
SELECT * FROM templates
ORDER BY (name, id) ASC
;

-- name: InsertTemplate :one
INSERT INTO
	templates (
		id,
		created_at,
		updated_at,
		organization_id,
		"name",
		provisioner,
		active_version_id,
		description,
		max_ttl,
		min_autostart_interval,
		created_by,
		icon
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: UpdateTemplateActiveVersionByID :exec
UPDATE
	templates
SET
	active_version_id = $2,
	updated_at = $3
WHERE
	id = $1;

-- name: UpdateTemplateDeletedByID :exec
UPDATE
	templates
SET
	deleted = $2,
	updated_at = $3
WHERE
	id = $1;

-- name: UpdateTemplateMetaByID :one
UPDATE
	templates
SET
	updated_at = $2,
	description = $3,
	max_ttl = $4,
	min_autostart_interval = $5,
	name = $6,
	icon = $7
WHERE
	id = $1
RETURNING
	*;

-- name: GetTemplatesAverageBuildTime :many
-- Computes average build time for every template.
-- Only considers last moving_average_size successful builds between start_ts and end_ts.
-- If a template does not have at least min_completed_job_count such builds, it gets skipped.
WITH query_with_all_job_count AS (SELECT
	DISTINCT t.id,
	AVG(pj.exec_time_sec)
		OVER(
			PARTITION BY t.id
			ORDER BY pj.completed_at
			ROWS BETWEEN @moving_average_size::integer PRECEDING AND CURRENT ROW)
		AS avg_build_time_sec,
	COUNT(*) OVER(PARTITION BY t.id) as job_count
FROM
	(SELECT
		id,
		active_version_id
	FROM
		templates) AS t
INNER JOIN
	(SELECT
		workspace_id,
		template_version_id,
		job_id
	FROM
		workspace_builds)
	AS
		wb
ON
	t.id = wb.workspace_id AND t.active_version_id = wb.template_version_id
INNER JOIN
	(SELECT
		id,
		completed_at,
		EXTRACT(EPOCH FROM (completed_at - started_at)) AS exec_time_sec
	FROM
		provisioner_jobs
	WHERE
		(completed_at IS NOT NULL) AND (started_at IS NOT NULL) AND
		(completed_at >= @start_ts AND completed_at <= @end_ts) AND
		(canceled_at IS NULL) AND
		((error IS NULL) OR (error = '')))
	AS
		pj
ON
	wb.job_id = pj.id)
SELECT
	id,
	avg_build_time_sec
FROM
	query_with_all_job_count
WHERE
	job_count >= @min_completed_job_count::integer
;
