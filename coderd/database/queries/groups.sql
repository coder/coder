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

-- name: GetGroupMembers :many
SELECT
	users.*
FROM
	users
JOIN
	group_members
ON
	users.id = group_members.user_id
WHERE
	group_members.group_id = $1
AND
	users.status = 'active'
AND
	users.deleted = 'false';

-- name: GetAllOrganizationMembers :many
SELECT
	users.*
FROM
	users
JOIN
	organization_members
ON
	users.id = organization_members.user_id
WHERE
	organization_members.organization_id = $1;

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
	( $1, $2, $3, $4, $5) RETURNING *;

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
	( sqlc.arg(organization_id), 'Everyone', sqlc.arg(organization_id)) RETURNING *;

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

-- name: InsertGroupMember :exec
INSERT INTO group_members (
	user_id,
	group_id
)
VALUES ( $1, $2);

-- name: DeleteGroupMember :exec
DELETE FROM
	group_members
WHERE
	user_id = $1;

-- name: DeleteGroupByID :exec
DELETE FROM
	groups
WHERE
	id = $1;


