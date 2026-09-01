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
		cached_module_files,
		updated_at,
	    provisionerd_version
	)
VALUES
	(
		(select id from template_versions where job_id = @job_id),
		@cached_plan,
		@cached_module_files,
		@updated_at,
		@provisionerd_version
	);

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
