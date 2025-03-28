-- name: InsertPreset :one
INSERT INTO template_version_presets (
	template_version_id,
	name,
	created_at,
	desired_instances,
	invalidate_after_secs
)
VALUES (
	@template_version_id,
	@name,
	@created_at,
	@desired_instances,
	@invalidate_after_secs
) RETURNING *;

-- name: InsertPresetParameters :many
INSERT INTO
	template_version_preset_parameters (template_version_preset_id, name, value)
SELECT
	@template_version_preset_id,
	unnest(@names :: TEXT[]),
	unnest(@values :: TEXT[])
RETURNING *;

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
	template_version_presets.template_version_id = @template_version_id
	AND (sqlc.narg('preset_id')::uuid IS NULL OR template_version_presets.id = sqlc.narg('preset_id'));
