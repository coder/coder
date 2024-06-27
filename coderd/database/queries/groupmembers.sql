-- name: GetGroupMembers :many
SELECT * FROM group_members;

-- name: GetGroupMembersByGroupID :many
SELECT
	users.*
FROM
	users
-- If the group is a user made group, then we need to check the group_members table.
LEFT JOIN
	group_members
ON
	group_members.user_id = users.id AND
	group_members.group_id = @group_id
-- If it is the "Everyone" group, then we need to check the organization_members table.
LEFT JOIN
	organization_members
ON
	organization_members.user_id = users.id AND
	organization_members.organization_id = @group_id
WHERE
	-- In either case, the group_id will only match an org or a group.
    (group_members.group_id = @group_id
         OR
     organization_members.organization_id = @group_id)
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

-- name: RemoveUserFromAllGroups :exec
DELETE FROM
	group_members
WHERE
	user_id = @user_id;

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
