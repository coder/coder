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
		WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
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
		default_ttl,
		created_by,
		icon,
		user_acl,
		group_acl
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING *;

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
	default_ttl = $4,
	name = $5,
	icon = $6
WHERE
	id = $1
RETURNING
	*;

-- name: UpdateTemplateACLByID :one
UPDATE
	templates
SET
	group_acl = $1,
	user_acl = $2
WHERE
	id = $3
RETURNING
	*;

-- name: GetTemplateAverageBuildTime :one
WITH build_times AS (
SELECT
	EXTRACT(EPOCH FROM (pj.completed_at - pj.started_at))::FLOAT AS exec_time_sec,
	workspace_builds.transition
FROM
	workspace_builds
JOIN template_versions ON
	workspace_builds.template_version_id = template_versions.id
JOIN provisioner_jobs pj ON
	workspace_builds.job_id = pj.id
WHERE
	template_versions.template_id = @template_id AND
		(pj.completed_at IS NOT NULL) AND (pj.started_at IS NOT NULL) AND
		(pj.started_at > @start_time) AND
		(pj.canceled_at IS NULL) AND
		((pj.error IS NULL) OR (pj.error = ''))
ORDER BY
	workspace_builds.created_at DESC
)
SELECT
	-- Postgres offers no clear way to DRY this short of a function or other
	-- complexities.
	coalesce((PERCENTILE_DISC(0.5) WITHIN GROUP(ORDER BY exec_time_sec) FILTER (WHERE transition = 'start')), -1)::FLOAT AS start_median,
	coalesce((PERCENTILE_DISC(0.5) WITHIN GROUP(ORDER BY exec_time_sec) FILTER (WHERE transition = 'stop')), -1)::FLOAT AS stop_median,
	coalesce((PERCENTILE_DISC(0.5) WITHIN GROUP(ORDER BY exec_time_sec) FILTER (WHERE transition = 'delete')), -1)::FLOAT AS delete_median
FROM build_times
;
