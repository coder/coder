-- name: GetFileByID :one
SELECT
	*
FROM
	files
WHERE
	id = $1
LIMIT
	1;

-- name: GetFileIDByTemplateVersionID :one
SELECT
	files.id
FROM
	files
JOIN
	provisioner_jobs ON
		provisioner_jobs.storage_method = 'file'
		AND provisioner_jobs.file_id = files.id
JOIN
	template_versions ON template_versions.job_id = provisioner_jobs.id
WHERE
	template_versions.id = @template_version_id
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
