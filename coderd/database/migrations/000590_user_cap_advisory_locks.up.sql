-- Move the per-user cap triggers (user_secrets, user_skills) off the users
-- row onto transaction-scoped per-user advisory locks, and close the
-- owner-reassignment bypass of the skills cap.
--
-- The old cap functions serialized on the users row with FOR UPDATE, which:
--   * conflicts with the FOR KEY SHARE locks foreign-key validation takes
--     on the users row for every table referencing users,
--   * can deadlock with any multi-row writer that touches child rows before
--     the users row (delete_deleted_user_resources deletes child rows of a
--     user inside the same statement that updated users), and
--   * is a stronger lock than counting requires.
--
-- Lock hygiene: the bodies are swapped with CREATE OR REPLACE FUNCTION and
-- the triggers renamed with ALTER TRIGGER ... RENAME, neither of which
-- takes ACCESS EXCLUSIVE on the tables; the one CREATE TRIGGER below takes
-- SHARE ROW EXCLUSIVE on user_skills only (blocks writes briefly, never
-- reads). No trigger is dropped.
--
-- The zz_ name prefix reserves BEFORE-trigger firing order (name order) so
-- that later-added row-guard triggers on these tables fire before the caps
-- take their advisory locks; a stacked change adds soft-delete guards that
-- lock the users row and must do so before the advisory lock to keep one
-- global lock order.
--
-- Isolation contract (stated, not enforced): the caps count committed
-- sibling rows under the advisory lock, which is race-free at READ
-- COMMITTED because the count re-reads the latest committed state after
-- the lock wait. Every production writer of these tables runs at the
-- default isolation level. A maintenance transaction running at
-- REPEATABLE READ (dbcrypt rotation rewrites user_secrets values under RR
-- and relies on 40001 retries) still serializes on the advisory lock but
-- counts from its own snapshot, so the byte caps are best-effort for such
-- writers, as they were under the old users-row lock. There is
-- deliberately no isolation-level trigger gate: a runtime gate would turn
-- a deployment-level default_transaction_isolation setting into a total
-- outage of secret and skill writes to prevent a bounded cap slip.

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
    -- The advisory lock avoids the users row entirely: no writer of other
    -- tables referencing users is affected, and no lock cycle through the
    -- users row is possible. The key derivation is registered for
    -- discoverability in coderd/database/lock.go and pinned by
    -- TestUserCapAdvisoryLocks.
    PERFORM pg_advisory_xact_lock(hashtextextended('user_secrets_cap:' || NEW.user_id::text, 0));

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
    -- Serialize skill-cap checks per user so concurrent inserts (or owner
    -- reassignments targeting the same user) cannot all observe the same
    -- pre-statement count and exceed the hard limit. See
    -- enforce_user_secrets_per_user_limits for why this is an advisory
    -- lock; the key registry is coderd/database/lock.go.
    PERFORM pg_advisory_xact_lock(hashtextextended('user_skills_cap:' || NEW.user_id::text, 0));

    -- On an owner reassignment the moving row still belongs to
    -- OLD.user_id, so counting NEW.user_id's rows excludes it naturally.
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

ALTER TRIGGER trigger_user_secrets_per_user_limits ON user_secrets
    RENAME TO trigger_zz_user_secrets_per_user_limits;
ALTER TRIGGER trigger_user_skills_per_user_limit ON user_skills
    RENAME TO trigger_zz_user_skills_per_user_limit;

-- The skills cap previously fired on INSERT only, so
-- UPDATE ... SET user_id could move a row onto an owner already at the
-- cap without a recount. The WHEN clause keeps same-owner updates (which
-- cannot change any per-user count) out of plpgsql entirely; the existing
-- INSERT trigger is untouched. No UPDATE leg is needed for user_secrets:
-- its existing trigger already fires BEFORE INSERT OR UPDATE and recounts
-- unconditionally, because a same-owner update can change the byte
-- aggregates.
CREATE TRIGGER trigger_zz_user_skills_per_user_limit_update
    BEFORE UPDATE ON user_skills
    FOR EACH ROW
    WHEN (NEW.user_id IS DISTINCT FROM OLD.user_id)
EXECUTE FUNCTION enforce_user_skills_per_user_limit();
