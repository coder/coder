-- name: InsertTemplateVersionTerraformValuesByJobID :exec
INSERT INTO
	template_version_terraform_values (
		template_version_id,
		cached_plan,
		updated_at
	)
VALUES
	(
		(select id from template_versions where job_id = @job_id),
		@cached_plan,
		@updated_at
	);
