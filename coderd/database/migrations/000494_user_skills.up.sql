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

-- Adds the user skill audit resource type.
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'user_skill';

-- Adds API key scopes for managing user skills.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_skill:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_skill:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_skill:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_skill:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_skill:*';
