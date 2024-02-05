-- name: GetTemplateByID :one
SELECT
	*
FROM
	template_with_users
WHERE
	id = $1
LIMIT
	1;

-- name: GetTemplatesWithFilter :many
SELECT
	*
FROM
	template_with_users AS templates
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
	-- Filter by deprecated
	AND CASE
		WHEN sqlc.narg('deprecated') :: boolean IS NOT NULL THEN
			CASE
				WHEN sqlc.narg('deprecated') :: boolean THEN
					deprecated != ''
				ELSE
					deprecated = ''
			END
		ELSE true
	END
  -- Authorize Filter clause will be injected below in GetAuthorizedTemplates
  -- @authorize_filter
ORDER BY (name, id) ASC
;

-- name: GetTemplateByOrganizationAndName :one
SELECT
	*
FROM
	template_with_users AS templates
WHERE
	organization_id = @organization_id
	AND deleted = @deleted
	AND LOWER("name") = LOWER(@name)
LIMIT
	1;

-- name: GetTemplates :many
SELECT * FROM template_with_users AS templates
ORDER BY (name, id) ASC
;

-- name: InsertTemplate :exec
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
		created_by,
		icon,
		user_acl,
		group_acl,
		display_name,
		allow_user_cancel_workspace_jobs
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

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

-- name: UpdateTemplateMetaByID :exec
UPDATE
	templates
SET
	updated_at = $2,
	description = $3,
	name = $4,
	icon = $5,
	display_name = $6,
	allow_user_cancel_workspace_jobs = $7,
	group_acl = $8
WHERE
	id = $1
;

-- name: UpdateTemplateScheduleByID :exec
UPDATE
	templates
SET
	updated_at = $2,
	allow_user_autostart = $3,
	allow_user_autostop = $4,
	default_ttl = $5,
	use_max_ttl = $6,
	max_ttl = $7,
	autostop_requirement_days_of_week = $8,
	autostop_requirement_weeks = $9,
	autostart_block_days_of_week = $10,
	failure_ttl = $11,
	time_til_dormant = $12,
	time_til_dormant_autodelete = $13
WHERE
	id = $1
;

-- name: UpdateTemplateACLByID :exec
UPDATE
	templates
SET
	group_acl = $1,
	user_acl = $2
WHERE
	id = $3
;

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
	coalesce((PERCENTILE_DISC(0.5) WITHIN GROUP(ORDER BY exec_time_sec) FILTER (WHERE transition = 'start')), -1)::FLOAT AS start_50,
	coalesce((PERCENTILE_DISC(0.5) WITHIN GROUP(ORDER BY exec_time_sec) FILTER (WHERE transition = 'stop')), -1)::FLOAT AS stop_50,
	coalesce((PERCENTILE_DISC(0.5) WITHIN GROUP(ORDER BY exec_time_sec) FILTER (WHERE transition = 'delete')), -1)::FLOAT AS delete_50,
	coalesce((PERCENTILE_DISC(0.95) WITHIN GROUP(ORDER BY exec_time_sec) FILTER (WHERE transition = 'start')), -1)::FLOAT AS start_95,
	coalesce((PERCENTILE_DISC(0.95) WITHIN GROUP(ORDER BY exec_time_sec) FILTER (WHERE transition = 'stop')), -1)::FLOAT AS stop_95,
	coalesce((PERCENTILE_DISC(0.95) WITHIN GROUP(ORDER BY exec_time_sec) FILTER (WHERE transition = 'delete')), -1)::FLOAT AS delete_95
FROM build_times
;

-- name: UpdateTemplateAccessControlByID :exec
UPDATE
	templates
SET
	require_active_version = $2,
	deprecated = $3
WHERE
	id = $1
;
