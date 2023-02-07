-- name: GetGroupByID :one
SELECT
	*
FROM
	groups
WHERE
	id = $1
LIMIT
	1;

-- name: GetGroupByOrgAndName :one
SELECT
	*
FROM
	groups
WHERE
	organization_id = $1
AND
	name = $2
LIMIT
	1;

-- name: GetGroupsByOrganizationID :many
SELECT
	*
FROM
	groups
WHERE
	organization_id = $1
AND
	id != $1;

-- name: InsertGroup :one
INSERT INTO groups (
	id,
	name,
	organization_id,
	avatar_url,
	quota_allowance
)
VALUES
	($1, $2, $3, $4, $5) RETURNING *;

-- We use the organization_id as the id
-- for simplicity since all users is
-- every member of the org.
-- name: InsertAllUsersGroup :one
INSERT INTO groups (
	id,
	name,
	organization_id
)
VALUES
	(sqlc.arg(organization_id), 'Everyone', sqlc.arg(organization_id)) RETURNING *;

-- name: UpdateGroupByID :one
UPDATE
	groups
SET
	name = $1,
	avatar_url = $2,
	quota_allowance = $3
WHERE
	id = $4
RETURNING *;

-- name: DeleteGroupByID :exec
DELETE FROM
	groups
WHERE
	id = $1;


