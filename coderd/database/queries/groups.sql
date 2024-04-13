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
	organization_id = $1;

-- name: GetGroupsByOrganizationAndUserID :many
SELECT
    groups.*
FROM
    groups
	-- If the group is a user made group, then we need to check the group_members table.
LEFT JOIN
    group_members
ON
    group_members.group_id = groups.id AND
    group_members.user_id = @user_id
	-- If it is the "Everyone" group, then we need to check the organization_members table.
LEFT JOIN
    organization_members
ON
    organization_members.organization_id = groups.id AND
    organization_members.user_id = @user_id
WHERE
    -- In either case, the group_id will only match an org or a group.
    (group_members.user_id = @user_id OR organization_members.user_id = @user_id)
AND
    -- Ensure the group or organization is the specified organization.
    groups.organization_id = @organization_id;


-- name: InsertGroup :one
INSERT INTO groups (
	id,
	name,
	display_name,
	organization_id,
	avatar_url,
	quota_allowance
)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: InsertMissingGroups :many
-- Inserts any group by name that does not exist. All new groups are given
-- a random uuid, are inserted into the same organization. They have the default
-- values for avatar, display name, and quota allowance (all zero values).
INSERT INTO groups (
	id,
	name,
	organization_id,
    source
)
SELECT
    gen_random_uuid(),
    group_name,
    @organization_id,
    @source
FROM
    UNNEST(@group_names :: text[]) AS group_name
-- If the name conflicts, do nothing.
ON CONFLICT DO NOTHING
RETURNING *;


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
	name = @name,
	display_name = @display_name,
	avatar_url = @avatar_url,
	quota_allowance = @quota_allowance
WHERE
	id = @id
RETURNING *;

-- name: DeleteGroupByID :exec
DELETE FROM
	groups
WHERE
	id = $1;


