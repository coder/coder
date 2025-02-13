CREATE TABLE IF NOT EXISTS user_configs (
		user_id uuid NOT NULL,
		key varchar(256) NOT NULL,
		value text NOT NULL
);

ALTER TABLE ONLY user_configs
    ADD CONSTRAINT unique_key_per_user UNIQUE (user_id, key);


--
INSERT INTO user_configs (user_id, key, value)
  SELECT id, 'theme_preference', theme_preference
    FROM users
    WHERE users.theme_preference IS NOT NULL;


-- Replace "group_members_expanded" without "theme_preference"
DROP VIEW group_members_expanded;
-- Taken from 000242_group_members_view.up.sql
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

-- Drop the "theme_preference" column now that the view no longer depends on it.
ALTER TABLE users DROP COLUMN theme_preference;
