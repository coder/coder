-- name: GetFileByID :one
SELECT
	*
FROM
	files
WHERE
	id = $1
LIMIT
	1;


-- name: GetFileByHashAndCreator :one
SELECT
	*
FROM
	files
WHERE
	hash = $1
AND
	created_by = $2
LIMIT
	1;


-- name: InsertFile :one
INSERT INTO
	files (id, hash, created_at, created_by, mimetype, "data")
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetFileTemplates :many
-- Get all templates that use a file.
SELECT
	files.id AS file_id,
	files.created_by AS file_created_by,
	templates.id AS template_id,
	templates.organization_id AS template_organization_id,
	templates.created_by AS template_created_by,
	templates.user_acl,
	templates.group_acl
FROM
	templates
INNER JOIN
	template_versions
	ON templates.id = template_versions.template_id
INNER JOIN
	provisioner_jobs
	ON job_id = provisioner_jobs.id
INNER JOIN
	files
	ON files.id = provisioner_jobs.file_id
WHERE
    -- Only fetch template version associated files.
	storage_method = 'file'
	AND provisioner_jobs.type = 'template_version_import'
	AND file_id = @file_id
;

-- name: DeleteCachedModuleFilesCreatedBetween :execrows
-- Deletes cached Terraform module archives ingested in the given time range and
-- clears the template version references to them. created_by and mimetype
-- identify a provisionerd-written module archive, matching the checks in
-- provisionerdserver, so user-uploaded template tarballs are never removed.
-- Only archives referenced by a template version are considered.
WITH doomed AS (
	SELECT
		files.id
	FROM
		files
	INNER JOIN
		template_version_terraform_values
		ON template_version_terraform_values.cached_module_files = files.id
	WHERE
		files.created_by = '00000000-0000-0000-0000-000000000000'
		AND files.mimetype = 'application/x-tar'
		AND files.created_at >= @created_at_after
		AND files.created_at < @created_at_before
), cleared AS (
	-- The foreign key is NO ACTION, so references must be cleared before the
	-- files rows can be deleted. Data-modifying CTEs always run to completion,
	-- and the constraint is checked at the end of the statement.
	UPDATE
		template_version_terraform_values
	SET
		cached_module_files = NULL
	WHERE
		cached_module_files IN (SELECT id FROM doomed)
	RETURNING 1
)
DELETE FROM
	files
USING
	doomed
WHERE
	files.id = doomed.id;
