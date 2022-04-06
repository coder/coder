-- name: GetWorkspaceByID :one
SELECT
	*
FROM
	workspaces
WHERE
	id = $1
LIMIT
	1;

-- name: GetWorkspacesByTemplateID :many
SELECT
	*
FROM
	workspaces
WHERE
	template_id = $1
	AND deleted = $2;

-- name: GetWorkspacesByUserID :many
SELECT
	*
FROM
	workspaces
WHERE
	owner_id = $1
	AND deleted = $2;

-- name: GetWorkspaceByUserIDAndName :one
SELECT
	*
FROM
	workspaces
WHERE
	owner_id = @owner_id
	AND deleted = @deleted
	AND LOWER("name") = LOWER(@name);

-- name: GetWorkspaceOwnerCountsByTemplateIDs :many
SELECT
	template_id,
	COUNT(DISTINCT owner_id)
FROM
	workspaces
WHERE
	template_id = ANY(@ids :: uuid [ ])
GROUP BY
	template_id,
	owner_id;

-- name: InsertWorkspace :one
INSERT INTO
	workspaces (
		id,
		created_at,
		updated_at,
		owner_id,
		template_id,
		name
	)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: UpdateWorkspaceDeletedByID :exec
UPDATE
	workspaces
SET
	deleted = $2
WHERE
	id = $1;
