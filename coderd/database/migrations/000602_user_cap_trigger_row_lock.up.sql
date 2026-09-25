-- Weaken the users-row lock taken by the per-user cap triggers on
-- user_secrets and user_skills from FOR UPDATE to FOR NO KEY UPDATE, and
-- recheck users.deleted after the lock wait.
--
-- FOR UPDATE also blocked the FOR KEY SHARE locks that foreign-key checks
-- take on the users row, so a cap write stalled inserts into every table
-- that references users for that user. FOR NO KEY UPDATE still serializes
-- cap writers for one user and still conflicts with the soft-delete UPDATE
-- of the users row.
--
-- Soft-delete ordering: if a cap write locks the users row first, the
-- soft-delete waits for it, and delete_deleted_user_resources then deletes
-- the committed row. If the soft-delete locks first, the cap write waits,
-- and the recheck rejects it. Before this migration the second order let
-- the write commit a row for a deleted user, because the deleted check ran
-- in an earlier trigger, before the wait.
--
-- CREATE OR REPLACE FUNCTION takes no lock on either table.

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
    -- Serialize cap checks per user so concurrent inserts or updates cannot
    -- all observe the same pre-statement aggregates and exceed the caps.
    -- FOR NO KEY UPDATE conflicts with itself and with the soft-delete
    -- UPDATE of the users row, but not with the FOR KEY SHARE locks that
    -- foreign-key checks take on the users row.
    PERFORM 1 FROM users WHERE id = NEW.user_id FOR NO KEY UPDATE;

    -- trigger_upsert_user_secrets checked users.deleted before this
    -- trigger could wait for the lock. Recheck now: at READ COMMITTED this
    -- statement sees a soft-delete that committed during the wait.
    IF (SELECT deleted FROM users WHERE id = NEW.user_id) THEN
        RAISE EXCEPTION 'Cannot create user_secret for deleted user';
    END IF;

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
    -- observe the same pre-insert count and exceed the hard limit. See
    -- enforce_user_secrets_per_user_limits for the lock strength.
    PERFORM 1
    FROM users
    WHERE id = NEW.user_id
    FOR NO KEY UPDATE;

    -- trigger_upsert_user_skills checked users.deleted before this trigger
    -- could wait for the lock. Recheck now: at READ COMMITTED this
    -- statement sees a soft-delete that committed during the wait.
    IF (SELECT deleted FROM users WHERE id = NEW.user_id) THEN
        RAISE EXCEPTION 'Cannot create user_skill for deleted user'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_skill_user_deleted';
    END IF;

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
