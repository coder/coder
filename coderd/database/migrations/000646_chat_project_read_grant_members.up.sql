-- Users that read grants on chat projects reach: granted users, and members
-- of granted groups including the Everyone group, who belong to the
-- project's organization. Only grants that include read count, as in
-- IsChatProjectUsableByUserID. The owner is not included, and a user
-- reached by several grants appears once per grant.
CREATE VIEW chat_project_read_grant_members AS
SELECT chat_projects.id AS project_id, organization_members.user_id
FROM chat_projects
CROSS JOIN LATERAL jsonb_each(chat_projects.user_acl) AS grants(principal, entry)
JOIN organization_members
    ON organization_members.user_id = grants.principal::uuid
    AND organization_members.organization_id = chat_projects.organization_id
WHERE grants.entry -> 'permissions' ?| ARRAY['read', '*']
UNION ALL
SELECT chat_projects.id AS project_id, group_members_expanded.user_id
FROM chat_projects
CROSS JOIN LATERAL jsonb_each(chat_projects.group_acl) AS grants(principal, entry)
JOIN group_members_expanded ON group_members_expanded.group_id = grants.principal::uuid
-- A granted group from another organization only reaches its members who
-- also belong to the project's organization.
JOIN organization_members
    ON organization_members.user_id = group_members_expanded.user_id
    AND organization_members.organization_id = chat_projects.organization_id
WHERE grants.entry -> 'permissions' ?| ARRAY['read', '*'];
