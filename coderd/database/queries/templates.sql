-- name: GetTemplateByID :one
SELECT
	*
FROM
	template_with_names
WHERE
	id = $1
LIMIT
	1;

-- name: GetTemplatesWithFilter :many
SELECT
	t.*
FROM
	template_with_names AS t
LEFT JOIN
	template_versions tv ON t.active_version_id = tv.id
WHERE
	-- Optionally include deleted templates
	t.deleted = @deleted
	-- Filter by organization_id
	AND CASE
		WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			t.organization_id = @organization_id
		ELSE true
	END
	-- Filter by exact name
	AND CASE
		WHEN @exact_name :: text != '' THEN
			LOWER(t.name) = LOWER(@exact_name)
		ELSE true
	END
	-- Filter by exact display name
	AND CASE
		WHEN @exact_display_name :: text != '' THEN
			LOWER(t.display_name) = LOWER(@exact_display_name)
		ELSE true
	END
	-- Filter by name, matching on substring
	AND CASE
		WHEN @fuzzy_name :: text != '' THEN
			lower(t.name) ILIKE '%' || lower(@fuzzy_name) || '%'
		ELSE true
	END
	-- Filter by display_name, matching on substring (fallback to name if display_name is empty)
	AND CASE
		WHEN @fuzzy_display_name :: text != '' THEN
			CASE
				WHEN t.display_name IS NOT NULL AND t.display_name != '' THEN
					lower(t.display_name) ILIKE '%' || lower(@fuzzy_display_name) || '%'
				ELSE
					-- Remove spaces if present since 't.name' cannot have any spaces
					lower(t.name) ILIKE '%' || REPLACE(lower(@fuzzy_display_name), ' ', '') || '%'
			END
		ELSE true
	END
	-- Filter by ids
	AND CASE
		WHEN array_length(@ids :: uuid[], 1) > 0 THEN
			t.id = ANY(@ids)
		ELSE true
	END
	-- Filter by deprecated
	AND CASE
		WHEN sqlc.narg('deprecated') :: boolean IS NOT NULL THEN
			CASE
				WHEN sqlc.narg('deprecated') :: boolean THEN
					t.deprecated != ''
				ELSE
					t.deprecated = ''
			END
		ELSE true
	END
	-- Filter by has_ai_task in latest version
	AND CASE
		WHEN sqlc.narg('has_ai_task') :: boolean IS NOT NULL THEN
			tv.has_ai_task = sqlc.narg('has_ai_task') :: boolean
		ELSE true
	END
	-- Filter by author_id
	AND CASE
		  WHEN @author_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			  t.created_by = @author_id
		  ELSE true
	END
	-- Filter by author_username
	AND CASE
		  WHEN @author_username :: text != '' THEN
			  t.created_by = (SELECT id FROM users WHERE lower(users.username) = lower(@author_username) AND deleted = false)
		  ELSE true
	END

	-- Filter by has_external_agent in latest version
	AND CASE
		WHEN sqlc.narg('has_external_agent') :: boolean IS NOT NULL THEN
			tv.has_external_agent = sqlc.narg('has_external_agent') :: boolean
		ELSE true
	END
  -- Authorize Filter clause will be injected below in GetAuthorizedTemplates
  -- @authorize_filter
ORDER BY (t.name, t.id) ASC
;

-- name: GetTemplateByOrganizationAndName :one
SELECT
	*
FROM
	template_with_names AS templates
WHERE
	organization_id = @organization_id
	AND deleted = @deleted
	AND LOWER("name") = LOWER(@name)
LIMIT
	1;

-- name: GetTemplates :many
SELECT * FROM template_with_names AS templates
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
		allow_user_cancel_workspace_jobs,
		max_port_sharing_level,
		use_classic_parameter_flow,
		cors_behavior
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17);

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
	group_acl = $8,
	max_port_sharing_level = $9,
	use_classic_parameter_flow = $10,
	cors_behavior = $11
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
	activity_bump = $6,
	autostop_requirement_days_of_week = $7,
	autostop_requirement_weeks = $8,
	autostart_block_days_of_week = $9,
	failure_ttl = $10,
	time_til_dormant = $11,
	time_til_dormant_autodelete = $12
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
		(pj.canceled_at IS NULL) AND
		((pj.error IS NULL) OR (pj.error = ''))
ORDER BY
	workspace_builds.created_at DESC
LIMIT 100
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
