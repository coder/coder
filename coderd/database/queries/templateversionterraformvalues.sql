-- name: GetTemplateVersionTerraformValues :one
SELECT
	template_version_terraform_values.*
FROM
	template_version_terraform_values
WHERE
	template_version_terraform_values.template_version_id = @template_version_id;

-- name: InsertTemplateVersionTerraformValuesByJobID :exec
INSERT INTO
	template_version_terraform_values (
		template_version_id,
		cached_plan,
		cached_plan_key_id,
		cached_module_files,
		updated_at,
	    provisionerd_version
	)
VALUES
	(
		(select id from template_versions where job_id = @job_id),
		@cached_plan,
		sqlc.narg('cached_plan_key_id'),
		@cached_module_files,
		@updated_at,
		@provisionerd_version
	);

-- name: UpdateTemplateVersionTerraformValuesCachedPlan :exec
-- Rewrites a cached plan in place. Used by `coder server dbcrypt`
-- maintenance commands to re-encrypt or decrypt the plan at rest.
UPDATE
	template_version_terraform_values
SET
	cached_plan = @cached_plan,
	cached_plan_key_id = sqlc.narg('cached_plan_key_id'),
	updated_at = @updated_at
WHERE
	template_version_id = @template_version_id;

-- name: GetTemplateVersionTerraformValuesCachedPlanKeyIDs :many
-- Returns a page of template versions that hold a cached plan across the
-- deployment, ordered by template version id, along with the key each plan
-- is stored with. Pass the last returned id as after_id to fetch the next
-- page. Used only by the dbcrypt key rotation utility.
SELECT
	template_version_id,
	cached_plan_key_id
FROM
	template_version_terraform_values
WHERE
	cached_plan IS NOT NULL
	AND cached_plan != 'null'::jsonb
	AND cached_plan != '{}'::jsonb
	AND template_version_id > @after_id::uuid
ORDER BY
	template_version_id
LIMIT
	@limit_count::int;

-- name: HasTemplateVersionsUsingCachedModuleFileInOrg :one
-- Reports whether the given file is referenced as cached module files by any
-- template version in the given organization. Used to authorize provisioner
-- module-file downloads so a daemon cannot read another organization's cached
-- Terraform module source.
SELECT EXISTS (
	SELECT 1
	FROM template_version_terraform_values tvtv
	JOIN template_versions tv
		ON tv.id = tvtv.template_version_id
	WHERE tvtv.cached_module_files = @file_id::uuid
		AND tv.organization_id = @organization_id::uuid
);
