-- name: InsertPreset :one
INSERT INTO template_version_presets (
	id,
	template_version_id,
	name,
	created_at,
	desired_instances,
	invalidate_after_secs,
	scheduling_timezone,
	is_default,
	description,
	icon,
	last_invalidated_at
)
VALUES (
	@id,
	@template_version_id,
	@name,
	@created_at,
	@desired_instances,
	@invalidate_after_secs,
	@scheduling_timezone,
	@is_default,
	@description,
	@icon,
	@last_invalidated_at
) RETURNING *;

-- name: InsertPresetParameters :many
INSERT INTO
	template_version_preset_parameters (template_version_preset_id, name, value)
SELECT
	@template_version_preset_id,
	unnest(@names :: TEXT[]),
	unnest(@values :: TEXT[])
RETURNING *;

-- name: InsertPresetPrebuildSchedule :one
INSERT INTO template_version_preset_prebuild_schedules (
	preset_id,
	cron_expression,
	desired_instances
)
VALUES (
	@preset_id,
	@cron_expression,
	@desired_instances
) RETURNING *;

-- name: UpdatePresetPrebuildStatus :exec
UPDATE template_version_presets
SET prebuild_status = @status
WHERE id = @preset_id;

-- name: GetPresetsByTemplateVersionID :many
SELECT
	*
FROM
	template_version_presets
WHERE
	template_version_id = @template_version_id;

-- name: GetPresetByWorkspaceBuildID :one
SELECT
	template_version_presets.*
FROM
	template_version_presets
	INNER JOIN workspace_builds ON workspace_builds.template_version_preset_id = template_version_presets.id
WHERE
	workspace_builds.id = @workspace_build_id;

-- name: GetPresetParametersByTemplateVersionID :many
SELECT
	template_version_preset_parameters.*
FROM
	template_version_preset_parameters
	INNER JOIN template_version_presets ON template_version_preset_parameters.template_version_preset_id = template_version_presets.id
WHERE
	template_version_presets.template_version_id = @template_version_id;

-- name: GetPresetParametersByPresetID :many
SELECT
	tvpp.*
FROM
	template_version_preset_parameters tvpp
WHERE
	tvpp.template_version_preset_id = @preset_id;

-- name: GetPresetByID :one
SELECT tvp.*, tv.template_id, tv.organization_id FROM
	template_version_presets tvp
	INNER JOIN template_versions tv ON tvp.template_version_id = tv.id
WHERE tvp.id = @preset_id;

-- name: GetActivePresetPrebuildSchedules :many
SELECT
	tvpps.*
FROM
	template_version_preset_prebuild_schedules tvpps
		INNER JOIN template_version_presets tvp ON tvp.id = tvpps.preset_id
		INNER JOIN template_versions tv ON tv.id = tvp.template_version_id
		INNER JOIN templates t ON t.id = tv.template_id
WHERE
	-- Template version is active, and template is not deleted or deprecated
	tv.id = t.active_version_id
	AND NOT t.deleted
	AND t.deprecated = '';

-- name: UpdatePresetsLastInvalidatedAt :many
UPDATE
	template_version_presets tvp
SET
	last_invalidated_at = @last_invalidated_at
FROM
	templates t
WHERE
	t.id = @template_id
	AND tvp.template_version_id = t.active_version_id
RETURNING *;
