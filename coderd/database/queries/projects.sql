-- name: GetProjectByID :one
SELECT
	*
FROM
	projects
WHERE
	id = $1
LIMIT
	1;

-- name: GetProjectsByIDs :many
SELECT
	*
FROM
	projects
WHERE
	id = ANY(@ids :: uuid [ ]);

-- name: GetProjectByOrganizationAndName :one
SELECT
	*
FROM
	projects
WHERE
	organization_id = @organization_id
	AND deleted = @deleted
	AND LOWER("name") = LOWER(@name)
LIMIT
	1;

-- name: GetProjectsByOrganization :many
SELECT
	*
FROM
	projects
WHERE
	organization_id = $1
	AND deleted = $2;

-- name: InsertProject :one
INSERT INTO
	projects (
		id,
		created_at,
		updated_at,
		organization_id,
		"name",
		provisioner,
		active_version_id
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: UpdateProjectActiveVersionByID :exec
UPDATE
	projects
SET
	active_version_id = $2
WHERE
	id = $1;

-- name: UpdateProjectDeletedByID :exec
UPDATE
	projects
SET
	deleted = $2
WHERE
	id = $1;
