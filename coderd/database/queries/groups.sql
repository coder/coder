-- name: GetGroupByID :one
SELECT
	*
FROM
	groups
WHERE
	id = $1
LIMIT
	1;

-- name: GetGroupByName :one
SELECT
	*
FROM
	groups
WHERE
	name = $1
LIMIT
	1;

-- name: GetUserGroups :many
SELECT
	groups.*
FROM
	groups
JOIN
	group_users
ON
	groups.id = group_users.group_id
WHERE
	group_users.user_id = $1;


-- name: GetGroupMembers :many
SELECT
	*
FROM
	users
JOIN
	group_users
ON
	users.id = group_users.user_id
WHERE
	group_users.group_id = $1;

-- name: GetGroupsByOrganizationID :many
SELECT
	*
FROM
	groups
WHERE
	organization_id = $1;
