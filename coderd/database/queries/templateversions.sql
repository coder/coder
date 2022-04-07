-- name: GetTemplateVersionsByTemplateID :many
SELECT
	*
FROM
	template_versions
WHERE
	template_id = $1 :: uuid;

-- name: GetTemplateVersionByJobID :one
SELECT
	*
FROM
	template_versions
WHERE
	job_id = $1;

-- name: GetTemplateVersionByTemplateIDAndName :one
SELECT
	*
FROM
	template_versions
WHERE
	template_id = $1
	AND "name" = $2;

-- name: GetTemplateVersionByID :one
SELECT
	*
FROM
	template_versions
WHERE
	id = $1;

-- name: InsertTemplateVersion :one
INSERT INTO
	template_versions (
		id,
		template_id,
		organization_id,
		created_at,
		updated_at,
		"name",
		description,
		job_id
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: UpdateTemplateVersionByID :exec
UPDATE
	template_versions
SET
	template_id = $2,
	updated_at = $3
WHERE
	id = $1;
