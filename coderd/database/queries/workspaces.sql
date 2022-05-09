-- name: GetWorkspaceByID :one
SELECT
	*
FROM
	workspaces
WHERE
	id = $1
LIMIT
	1;

-- name: GetWorkspacesByOrganizationID :many
SELECT * FROM workspaces WHERE organization_id = $1 AND deleted = $2;

-- name: GetWorkspacesByTemplateID :many
SELECT
	*
FROM
	workspaces
WHERE
	template_id = $1
	AND deleted = $2;

-- name: GetWorkspacesByOwnerID :many
SELECT
	*
FROM
	workspaces
WHERE
	owner_id = $1
	AND deleted = $2;

-- name: GetWorkspaceByOwnerIDAndName :one
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
		organization_id,
		template_id,
		name
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: UpdateWorkspaceDeletedByID :exec
UPDATE
	workspaces
SET
	deleted = $2
WHERE
	id = $1;

-- name: UpdateWorkspaceAutostart :exec
UPDATE
	workspaces
SET
	autostart_schedule = $2
WHERE
	id = $1;

-- name: UpdateWorkspaceAutostop :exec
UPDATE
	workspaces
SET
	autostop_schedule = $2
WHERE
	id = $1;
