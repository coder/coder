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
	    tfstate,
		updated_at
	)
VALUES
	(
		(select id from template_versions where job_id = @job_id),
		@cached_plan,
	 	@tfstate,
		@updated_at
	);
