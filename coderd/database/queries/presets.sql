-- name: InsertPreset :one
INSERT INTO
	template_version_presets (template_version_id, name, created_at, updated_at)
VALUES
	(@template_version_id, @name, @created_at, @updated_at) RETURNING *;

-- InsertPresetParameter :one
INSERT INTO
	template_version_preset_parameters (template_version_preset_id, name, value)
VALUES
	(@template_version_preset_id, @name, @value) RETURNING *;

-- name: GetPresetsByTemplateVersionID :many
SELECT
	id,
	name,
	created_at,
	updated_at
FROM
	template_version_presets
WHERE
	template_version_id = @template_version_id;

-- name: GetPresetByWorkspaceBuildID :one
SELECT
	template_version_presets.id,
	template_version_presets.name,
	template_version_presets.created_at,
	template_version_presets.updated_at
FROM
	workspace_builds
	LEFT JOIN template_version_presets ON workspace_builds.template_version_preset_id = template_version_presets.id
WHERE
	workspace_builds.id = @workspace_build_id;

-- name: GetPresetParametersByPresetID :many
SELECT
	id,
	name,
	value
FROM
	template_version_preset_parameters
WHERE
	template_version_preset_id = @template_version_preset_id;
