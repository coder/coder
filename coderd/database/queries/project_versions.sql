-- name: GetProjectVersionsByProjectID :many
SELECT
	*
FROM
	project_versions
WHERE
	project_id = $1 :: uuid;

-- name: GetProjectVersionByJobID :one
SELECT
	*
FROM
	project_versions
WHERE
	job_id = $1;

-- name: GetProjectVersionByProjectIDAndName :one
SELECT
	*
FROM
	project_versions
WHERE
	project_id = $1
	AND "name" = $2;

-- name: GetProjectVersionByID :one
SELECT
	*
FROM
	project_versions
WHERE
	id = $1;

-- name: InsertProjectVersion :one
INSERT INTO
	project_versions (
		id,
		project_id,
		organization_id,
		created_at,
		updated_at,
		"name",
		description,
		job_id
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: UpdateProjectVersionByID :exec
UPDATE
	project_versions
SET
	project_id = $2,
	updated_at = $3
WHERE
	id = $1;
