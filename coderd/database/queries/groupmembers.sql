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

-- InsertUserGroupsByName adds a user to all provided groups, if they exist.
-- name: InsertUserGroupsByName :exec
WITH groups AS (
    SELECT
        id
    FROM
        groups
    WHERE
        groups.organization_id = @organization_id AND
        groups.name = ANY(@group_names :: text [])
)
INSERT INTO
    group_members (user_id, group_id)
SELECT
    @user_id,
    groups.id
FROM
    groups;

-- name: DeleteGroupMembersByOrgAndUser :exec
DELETE FROM
    group_members
USING
    group_members AS gm
LEFT JOIN
    groups
ON
    groups.id = gm.group_id
WHERE
    groups.organization_id = @organization_id AND
    gm.user_id = @user_id;

-- name: InsertGroupMember :exec
INSERT INTO
    group_members (user_id, group_id)
VALUES
    ($1, $2);

-- name: DeleteGroupMemberFromGroup :exec
DELETE FROM
	group_members
WHERE
	user_id = $1 AND
	group_id = $2;
