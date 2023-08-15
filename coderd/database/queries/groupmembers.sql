-- name: GetGroupMembers :many
SELECT
	users.*
FROM
	users
LEFT JOIN
	group_members
ON
	CASE WHEN @id:: uuid != @organization_id :: uuid THEN
	group_members.user_id = users.id
	END
LEFT JOIN
	organization_members
ON
    CASE WHEN @id :: uuid = @organization_id :: uuid THEN
        organization_members.user_id = users.id
    END
WHERE
    CASE WHEN @id :: uuid != @organization_id :: uuid THEN
        group_members.group_id = @id
    ELSE true END
AND
    CASE WHEN @id :: uuid = @organization_id :: uuid THEN
        organization_members.organization_id = @organization_id
    ELSE true END
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
WHERE
	group_members.user_id = @user_id
	AND group_id = ANY(SELECT id FROM groups WHERE organization_id = @organization_id);

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
