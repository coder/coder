CREATE VIEW
    group_members_expanded
AS
-- If the group is a user made group, then we need to check the group_members table.
-- If it is the "Everyone" group, then we need to check the organization_members table.
WITH all_members AS (
    SELECT user_id, group_id FROM group_members
    UNION
    SELECT user_id, organization_id AS group_id FROM organization_members
)
SELECT
    users.id AS user_id,
    users.username,
    users.avatar_url AS user_avatar_url,
    groups.organization_id AS organization_id,
    groups.name AS group_name,
    all_members.group_id AS group_id
FROM
    all_members
JOIN
    users ON users.id = all_members.user_id
JOIN
    groups ON groups.id = all_members.group_id
WHERE
    users.deleted = 'false';

COMMENT ON VIEW group_members_expanded IS 'Joins group members with user information, organization ID, group name. Includes both regular group members and organization members (as part of the "Everyone" group).';
