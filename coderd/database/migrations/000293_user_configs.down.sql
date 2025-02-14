-- Put back "theme_preference" column
ALTER TABLE users ADD COLUMN IF NOT EXISTS
  theme_preference text DEFAULT ''::text NOT NULL;

-- Copy "theme_preference" back to "users"
UPDATE users (theme_preference)
  SELECT value
    FROM user_configs
    WHERE users.id = user_configs.user_id
		  AND user_configs.key = 'theme_preference';

-- Drop the "user_configs" table.
DROP TABLE user_configs;

-- Replace "group_members_expanded", and bring back with "theme_preference"
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
	users.theme_preference AS user_theme_preference,
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
