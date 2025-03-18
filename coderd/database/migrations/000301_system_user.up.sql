-- As per create_migration.sh, all migrations run in a single transaction.
-- This necessitates specific care be taken when modifying enums.
-- migration 000126 did not follow this pattern when it introduced the
-- "none" login type. Because we need the "none" login type in this migration,
-- we need to recreate the login_type enum here. By now, it has quite a few
-- dependencies, all of which must be recreated.
DROP VIEW IF EXISTS group_members_expanded;

CREATE TYPE new_logintype AS ENUM (
    'password',
    'github',
    'oidc',
    'token',
    'none',
		'oauth2_provider_app'
);
COMMENT ON TYPE new_logintype IS 'Specifies the method of authentication. "none" is a special case in which no authentication method is allowed.';

ALTER TABLE users
	ALTER COLUMN login_type DROP DEFAULT, -- if the column has a default, it must be dropped first
	ALTER COLUMN login_type TYPE new_logintype USING (login_type::text::new_logintype), -- converts the old enum into the new enum using text as an intermediary
	ALTER COLUMN login_type SET DEFAULT 'password'::new_logintype; -- re-add the default using the new enum

DROP INDEX IF EXISTS idx_api_key_name;
ALTER TABLE api_keys
	ALTER COLUMN login_type TYPE new_logintype USING (login_type::text::new_logintype); -- converts the old enum into the new enum using text as an intermediary
CREATE UNIQUE INDEX idx_api_key_name
ON api_keys (user_id, token_name)
WHERE (login_type = 'token'::new_logintype);

ALTER TABLE user_links
	ALTER COLUMN login_type TYPE new_logintype USING (login_type::text::new_logintype); -- converts the old enum into the new enum using text as an intermediary

DROP TYPE login_type;
ALTER TYPE new_logintype RENAME TO login_type;

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

-- Now on to the actual system user logic:

ALTER TABLE users
	ADD COLUMN is_system bool DEFAULT false NOT NULL;

CREATE INDEX user_is_system_idx ON users USING btree (is_system);

COMMENT ON COLUMN users.is_system IS 'Determines if a user is a system user, and therefore cannot login or perform normal actions';

INSERT INTO users (id, email, username, name, created_at, updated_at, status, rbac_roles, hashed_password, is_system, login_type)
VALUES ('c42fdf75-3097-471c-8c33-fb52454d81c0', 'prebuilds@system', 'prebuilds', 'Prebuilds Owner', now(), now(),
		'active', '{}', 'none', true, 'none'::login_type);

-- Create function to check system user modifications
CREATE OR REPLACE FUNCTION prevent_system_user_changes()
	RETURNS TRIGGER AS
$$
BEGIN
	IF OLD.is_system = true THEN
		RAISE EXCEPTION 'Cannot modify or delete system users';
	END IF;
	RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- Create trigger to prevent updates to system users
CREATE TRIGGER prevent_system_user_updates
	BEFORE UPDATE ON users
	FOR EACH ROW
	WHEN (OLD.is_system = true)
EXECUTE FUNCTION prevent_system_user_changes();

-- Create trigger to prevent deletion of system users
CREATE TRIGGER prevent_system_user_deletions
	BEFORE DELETE ON users
	FOR EACH ROW
	WHEN (OLD.is_system = true)
EXECUTE FUNCTION prevent_system_user_changes();

-- TODO: do we *want* to use the default org here? how do we handle multi-org?
WITH default_org AS (SELECT id
					 FROM organizations
					 WHERE is_default = true
					 LIMIT 1)
INSERT
INTO organization_members (organization_id, user_id, created_at, updated_at)
SELECT default_org.id,
	   'c42fdf75-3097-471c-8c33-fb52454d81c0', -- The system user responsible for prebuilds.
	   NOW(),
	   NOW()
FROM default_org;
