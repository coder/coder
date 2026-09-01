DROP TRIGGER IF EXISTS trigger_zz_user_skills_per_user_limit_update ON user_skills;

ALTER TRIGGER trigger_zz_user_secrets_per_user_limits ON user_secrets
    RENAME TO trigger_user_secrets_per_user_limits;
ALTER TRIGGER trigger_zz_user_skills_per_user_limit ON user_skills
    RENAME TO trigger_user_skills_per_user_limit;

-- Restore the users-row-locking bodies from migrations 000509 (user_secrets)
-- and 000502 (user_skills).
CREATE OR REPLACE FUNCTION enforce_user_secrets_per_user_limits() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    existing_count       int;
    existing_total_bytes bigint;
    existing_env_bytes   bigint;

    new_count       int;
    new_total_bytes bigint;
    new_env_bytes   bigint;

    count_limit       constant int    := 50;
    total_bytes_limit constant bigint := 204800;   -- 200 KiB
    env_bytes_limit   constant bigint := 24576;    -- 24 KiB
BEGIN
    -- Serialize cap checks per user so concurrent inserts cannot all
    -- observe the same pre-insert aggregates and exceed the cap.
    PERFORM 1 FROM users WHERE id = NEW.user_id FOR UPDATE;

    -- Sum existing rows excluding the row being updated (so UPDATE statements
    -- don't double-count NEW). On INSERT, no row matches NEW.id, so
    -- the FILTER is a no-op.
    SELECT
        count(*) FILTER (WHERE id IS DISTINCT FROM NEW.id),
        coalesce(sum(octet_length(value)) FILTER (WHERE id IS DISTINCT FROM NEW.id), 0),
        coalesce(sum(octet_length(value)) FILTER (WHERE id IS DISTINCT FROM NEW.id AND env_name <> ''), 0)
    INTO existing_count, existing_total_bytes, existing_env_bytes
    FROM user_secrets
    WHERE user_id = NEW.user_id;

    new_count       := existing_count + 1;
    new_total_bytes := existing_total_bytes + octet_length(NEW.value);
    new_env_bytes   := existing_env_bytes
                       + CASE WHEN NEW.env_name <> '' THEN octet_length(NEW.value) ELSE 0 END;

    IF new_count > count_limit THEN
        RAISE EXCEPTION 'user has reached the user secrets count limit (% > %)',
            new_count, count_limit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_secrets_per_user_count_limit';
    END IF;

    IF new_total_bytes > total_bytes_limit THEN
        RAISE EXCEPTION 'user has reached the user secrets total value bytes limit (% > %)',
            new_total_bytes, total_bytes_limit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_secrets_per_user_total_bytes_limit';
    END IF;

    IF new_env_bytes > env_bytes_limit THEN
        RAISE EXCEPTION 'user has reached the env-injected user secrets bytes limit (% > %)',
            new_env_bytes, env_bytes_limit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_secrets_per_user_env_bytes_limit';
    END IF;

    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION enforce_user_skills_per_user_limit() RETURNS trigger
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
