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
