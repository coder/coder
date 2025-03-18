ALTER TABLE users
	ADD COLUMN is_system bool DEFAULT false NOT NULL;

CREATE INDEX user_is_system_idx ON users USING btree (is_system);

COMMENT ON COLUMN users.is_system IS 'Determines if a user is a system user, and therefore cannot login or perform normal actions';

INSERT INTO users (id, email, username, name, created_at, updated_at, status, rbac_roles, hashed_password, is_system, login_type)
VALUES ('c42fdf75-3097-471c-8c33-fb52454d81c0', 'prebuilds@system', 'none', 'Prebuilds Owner', now(), now(),
		'active', '{}', 'none', true, 'password'::login_type);

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
