-- Creates the user_skills table and indexes.
CREATE TABLE user_skills (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX user_skills_user_id_name_idx ON user_skills (user_id, name);

-- Enforces the per-user personal-skill cap at the schema level so the
-- invariant survives any future refactor of InsertUserSkill. The cap
-- value must stay in sync with skills.MaxPersonalSkillsPerUser in Go.
CREATE FUNCTION enforce_user_skills_per_user_limit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    skill_count int;
    skill_limit constant int := 100;
BEGIN
    -- Serialize skill-cap checks per user so concurrent inserts cannot all
    -- observe the same pre-insert count and exceed the hard limit.
    PERFORM 1
    FROM users
    WHERE id = NEW.user_id
    FOR UPDATE;

    SELECT count(*) INTO skill_count
    FROM user_skills
    WHERE user_id = NEW.user_id;
    IF skill_count >= skill_limit THEN
        RAISE EXCEPTION 'user has reached the personal skill limit'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_skills_per_user_limit';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_user_skills_per_user_limit
BEFORE INSERT ON user_skills
FOR EACH ROW
EXECUTE PROCEDURE enforce_user_skills_per_user_limit();

-- Extend the soft-delete cleanup trigger to also wipe user_skills.
-- user_skills.user_id has ON DELETE CASCADE, but Coder soft-deletes
-- users by flipping users.deleted instead of removing the row, so the
-- FK cascade never fires and skills would otherwise survive deletion.
DELETE FROM
    user_skills
WHERE
    user_id
        IN (
        SELECT id FROM users WHERE deleted
    );

CREATE OR REPLACE FUNCTION delete_deleted_user_resources() RETURNS trigger
    LANGUAGE plpgsql
AS $$
DECLARE
BEGIN
    IF (NEW.deleted) THEN
        -- Remove their api_keys.
        DELETE FROM api_keys
        WHERE user_id = OLD.id;

        -- Remove their user_links.
        -- Their login_type is preserved in the users table.
        -- Matching this user back to the link can still be done by their
        -- email if the account is undeleted. Although that is not a guarantee.
        DELETE FROM user_links
        WHERE user_id = OLD.id;

        -- Remove their user_secrets.
        -- user_secrets.user_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM user_secrets
        WHERE user_id = OLD.id;

        -- Remove their organization memberships.
        -- This also triggers group membership cleanup via
        -- trigger_delete_group_members_on_org_member_delete.
        DELETE FROM organization_members
        WHERE user_id = OLD.id;

        -- Remove their user_skills.
        -- user_skills.user_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM user_skills
        WHERE user_id = OLD.id;
    END IF;
    RETURN NEW;
END;
$$;

-- Prevent adding new user_skills for soft-deleted users.
-- Closes the window between an in-flight CreateUserSkill request and
-- the soft-delete UPDATE committing.
CREATE FUNCTION insert_user_skill_fail_if_user_deleted() RETURNS trigger
    LANGUAGE plpgsql
AS $$

DECLARE
BEGIN
    IF (NEW.user_id IS NOT NULL) THEN
        IF (SELECT deleted FROM users WHERE id = NEW.user_id LIMIT 1) THEN
            RAISE EXCEPTION 'Cannot create user_skill for deleted user';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_upsert_user_skills
    BEFORE INSERT OR UPDATE ON user_skills
    FOR EACH ROW
EXECUTE PROCEDURE insert_user_skill_fail_if_user_deleted();

-- Adds the user skill audit resource type.
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'user_skill';

-- Adds API key scopes for managing user skills.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_skill:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_skill:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_skill:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_skill:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_skill:*';
