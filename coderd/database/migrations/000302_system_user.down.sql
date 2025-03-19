DROP VIEW IF EXISTS group_members_expanded;
CREATE VIEW group_members_expanded AS
 WITH all_members AS (
         SELECT group_members.user_id,
            group_members.group_id
           FROM group_members
        UNION
         SELECT organization_members.user_id,
            organization_members.organization_id AS group_id
           FROM organization_members
        )
 SELECT users.id AS user_id,
    users.email AS user_email,
    users.username AS user_username,
    users.hashed_password AS user_hashed_password,
    users.created_at AS user_created_at,
    users.updated_at AS user_updated_at,
    users.status AS user_status,
    users.rbac_roles AS user_rbac_roles,
    users.login_type AS user_login_type,
    users.avatar_url AS user_avatar_url,
    users.deleted AS user_deleted,
    users.last_seen_at AS user_last_seen_at,
    users.quiet_hours_schedule AS user_quiet_hours_schedule,
    users.name AS user_name,
    users.github_com_user_id AS user_github_com_user_id,
    groups.organization_id,
    groups.name AS group_name,
    all_members.group_id
   FROM ((all_members
     JOIN users ON ((users.id = all_members.user_id)))
     JOIN groups ON ((groups.id = all_members.group_id)))
  WHERE (users.deleted = false);

COMMENT ON VIEW group_members_expanded IS 'Joins group members with user information, organization ID, group name. Includes both regular group members and organization members (as part of the "Everyone" group).';

-- Remove system user from organizations
DELETE FROM organization_members
WHERE user_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0';

-- Delete user status changes
DELETE FROM user_status_changes
WHERE user_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0';

-- Delete system user
DELETE FROM users
WHERE id = 'c42fdf75-3097-471c-8c33-fb52454d81c0';

-- Drop index
DROP INDEX IF EXISTS user_is_system_idx;

-- Drop column
ALTER TABLE users DROP COLUMN IF EXISTS is_system;
