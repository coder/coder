-- Close the insert-vs-soft-delete race for every table that
-- delete_deleted_user_resources deletes directly: an in-flight insert could
-- observe deleted = false, a concurrent soft-delete UPDATE (and its cleanup)
-- could commit, and the insert could then commit afterwards, resurrecting a
-- row for a soft-deleted user. For api_keys that resurrects a live session
-- token on an account the operator believes they deleted.
--
-- The guard and lock-ordering rationale lives inside check_user_not_deleted()
-- below so it survives into dump.sql. This migration:
--
--  1. Replaces the bodies of the four per-table guard functions (api_keys,
--     user_links, user_secrets, user_skills) with delegation to one shared
--     check_user_not_deleted() function, so the lock and the operation gate
--     exist in exactly one place. CREATE OR REPLACE FUNCTION swaps the
--     bodies without touching the triggers: no DROP TRIGGER, no
--     ACCESS EXCLUSIVE lock on the hot tables during the upgrade.
--  2. Adds guard triggers to the directly cleaned tables that had none
--     (user_ai_provider_keys, organization_members) plus
--     user_ai_budget_overrides and group_members, whose rows feed readers
--     that do not filter users.deleted (GetOverBudgetUsersPerGroup reads
--     override rows into a budget metric; GetAuthorizationUserRoles reads
--     group_members directly into rbac.Subject.Groups). CREATE TRIGGER
--     takes SHARE ROW EXCLUSIVE only (blocks writes briefly, never reads).
--  3. Adds owner-reassignment coverage: every guarded table also checks
--     UPDATE ... SET user_id, so a live child row cannot be re-parented
--     onto a soft-deleted user. The users-row lock is taken exactly when
--     the row starts belonging to the target user (INSERT or owner
--     reassignment); same-owner updates keep the unlocked read.
--
-- There are deliberately no backfill DELETE statements here: unbounded deletes inside
-- the single-transaction migration would hold locks fleet-wide during the
-- upgrade. Orphaned child rows of already-soft-deleted users are removed by
-- the idempotent dbpurge reaper (PurgeSoftDeletedUserResources), which runs
-- at startup and periodically; the triggers installed here guarantee no new
-- orphans are created once this migration commits.

-- The shared soft-delete guard. take_lock selects the locking read (INSERT
-- and owner-reassignment paths); tg_operation feeds the error message;
-- display_name and deleted_constraint parameterize the raised error, with
-- the constraint name being the stable identifier API handlers match on.
CREATE FUNCTION check_user_not_deleted(
    target_user_id uuid,
    take_lock boolean,
    tg_operation text,
    display_name text,
    deleted_constraint text
) RETURNS void
    LANGUAGE plpgsql
AS $$
DECLARE
	user_deleted boolean;
BEGIN
	-- Serialize child-table inserts against a concurrent user soft-delete:
	-- an unlocked insert can read deleted = false, lose the race to the
	-- soft-delete UPDATE and its cleanup, then commit a resurrected row.
	-- FOR NO KEY UPDATE conflicts with the soft-delete UPDATE but not with
	-- the FOR KEY SHARE locks that foreign-key validation takes on the
	-- users row for other child tables.
	--
	-- The lock is taken only when the row starts belonging to
	-- target_user_id (INSERT, or UPDATE that reassigns the owner). On
	-- same-owner UPDATE statements the unlocked read is kept: taking the users lock
	-- there deadlocks against delete_deleted_user_resources (a multi-row
	-- UPDATE or ON CONFLICT path can hold one child tuple and wait on
	-- users while the cleanup holds users and waits on a child tuple), and
	-- would serialize routine child updates on the hot users row. An
	-- existing row is cleaned up by the soft-delete either way.
	--
	-- The locking path imposes an ordering contract on writers: a
	-- transaction that writes a guarded child row (INSERT, UPDATE, or
	-- DELETE) and later inserts a guarded row for the same user must call
	-- AcquireUserSoftDeleteGuardLock first, so its lock order (users, then
	-- child rows) matches delete_deleted_user_resources. The same contract
	-- covers the cap triggers' advisory locks (migration 000590): without
	-- the users lock first, an update-then-insert writer can cycle with a
	-- concurrent insert that holds the users lock and waits on the
	-- advisory lock. coderd/database/user_soft_delete_guards_test.go
	-- replays each known such path as a deterministic deadlock regression,
	-- and the OAuth2 token exchange is driven through its real entry point.
	--
	-- Isolation: the locking read is correct at READ COMMITTED, which every
	-- production writer of the guarded tables uses. Under REPEATABLE READ
	-- or SERIALIZABLE the lock wait fails with a serialization error
	-- (40001) whenever any transaction committed an update to the users
	-- row after the snapshot (users is written on ordinary browsing to
	-- bump last_seen_at), and coderd does not retry 40001; do not wrap
	-- guarded inserts in database.ReadModifyUpdate or other
	-- stronger-isolation transactions.
	IF take_lock THEN
		SELECT deleted INTO user_deleted
		FROM users
		WHERE id = target_user_id
		FOR NO KEY UPDATE;
	ELSE
		SELECT deleted INTO user_deleted
		FROM users
		WHERE id = target_user_id;
	END IF;
	IF (user_deleted) THEN
		RAISE EXCEPTION 'Cannot % % for deleted user',
			CASE WHEN tg_operation = 'INSERT' THEN 'create' ELSE 'modify' END,
			display_name
			USING ERRCODE = 'check_violation',
				  CONSTRAINT = deleted_constraint;
	END IF;
END;
$$;

-- The generic trigger form for tables whose triggers this migration
-- creates. TG_ARGV[0] is the display name for the error message, TG_ARGV[1]
-- the stable constraint name callers match on.
CREATE FUNCTION fail_if_user_deleted() RETURNS trigger
	LANGUAGE plpgsql
AS $$
DECLARE
	reassigned boolean := false;
BEGIN
	IF (TG_OP = 'UPDATE') THEN
		reassigned := NEW.user_id IS DISTINCT FROM OLD.user_id;
		-- Same-owner updates on the tables using this function are
		-- filtered out by the triggers' UPDATE OF user_id column list;
		-- reassigned still guards the no-op UPDATE ... SET user_id = user_id.
	END IF;
	PERFORM check_user_not_deleted(
		NEW.user_id, TG_OP = 'INSERT' OR reassigned, TG_OP, TG_ARGV[0], TG_ARGV[1]);
	RETURN NEW;
END;
$$;

-- Delegate the four pre-existing per-table guard functions to the shared
-- check. Their triggers are untouched; the INSERT-only tables additionally
-- get an owner-reassignment trigger below.
CREATE OR REPLACE FUNCTION insert_apikey_fail_if_user_deleted() RETURNS trigger
	LANGUAGE plpgsql
AS $$
DECLARE
	reassigned boolean := false;
BEGIN
	IF (TG_OP = 'UPDATE') THEN
		reassigned := NEW.user_id IS DISTINCT FROM OLD.user_id;
	END IF;
	PERFORM check_user_not_deleted(
		NEW.user_id, TG_OP = 'INSERT' OR reassigned, TG_OP, 'API key', 'api_key_user_deleted');
	RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION insert_user_links_fail_if_user_deleted() RETURNS trigger
	LANGUAGE plpgsql
AS $$
DECLARE
	reassigned boolean := false;
BEGIN
	IF (TG_OP = 'UPDATE') THEN
		reassigned := NEW.user_id IS DISTINCT FROM OLD.user_id;
	END IF;
	PERFORM check_user_not_deleted(
		NEW.user_id, TG_OP = 'INSERT' OR reassigned, TG_OP, 'user_link', 'user_link_user_deleted');
	RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION insert_user_secret_fail_if_user_deleted() RETURNS trigger
	LANGUAGE plpgsql
AS $$
DECLARE
	reassigned boolean := false;
BEGIN
	IF (TG_OP = 'UPDATE') THEN
		reassigned := NEW.user_id IS DISTINCT FROM OLD.user_id;
	END IF;
	PERFORM check_user_not_deleted(
		NEW.user_id, TG_OP = 'INSERT' OR reassigned, TG_OP, 'user_secret', 'user_secret_user_deleted');
	RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION insert_user_skill_fail_if_user_deleted() RETURNS trigger
	LANGUAGE plpgsql
AS $$
DECLARE
	reassigned boolean := false;
BEGIN
	IF (TG_OP = 'UPDATE') THEN
		reassigned := NEW.user_id IS DISTINCT FROM OLD.user_id;
	END IF;
	PERFORM check_user_not_deleted(
		NEW.user_id, TG_OP = 'INSERT' OR reassigned, TG_OP, 'user_skill', 'user_skill_user_deleted');
	RETURN NEW;
END;
$$;

-- Owner-reassignment coverage for api_keys, whose existing trigger is
-- INSERT-only. The UPDATE OF user_id column list plus the WHEN clause keep
-- every ordinary api_keys update (last_used bumps on each authenticated
-- request) out of plpgsql entirely.
CREATE TRIGGER trigger_update_apikeys_owner
	BEFORE UPDATE OF user_id ON api_keys
	FOR EACH ROW
	WHEN (NEW.user_id IS DISTINCT FROM OLD.user_id)
EXECUTE FUNCTION insert_apikey_fail_if_user_deleted();

-- Guards for the directly cleaned tables that had none, plus the two whose
-- rows feed deleted-user-blind readers (see the header).
CREATE TRIGGER trigger_insert_user_ai_provider_keys
	BEFORE INSERT OR UPDATE OF user_id ON user_ai_provider_keys
	FOR EACH ROW
EXECUTE FUNCTION fail_if_user_deleted('user_ai_provider_key', 'user_ai_provider_key_user_deleted');

CREATE TRIGGER trigger_insert_organization_members
	BEFORE INSERT OR UPDATE OF user_id ON organization_members
	FOR EACH ROW
EXECUTE FUNCTION fail_if_user_deleted('organization_member', 'organization_member_user_deleted');

CREATE TRIGGER trigger_insert_user_ai_budget_overrides
	BEFORE INSERT OR UPDATE OF user_id ON user_ai_budget_overrides
	FOR EACH ROW
EXECUTE FUNCTION fail_if_user_deleted('user_ai_budget_override', 'user_ai_budget_override_user_deleted');

CREATE TRIGGER trigger_insert_group_members
	BEFORE INSERT OR UPDATE OF user_id ON group_members
	FOR EACH ROW
EXECUTE FUNCTION fail_if_user_deleted('group_member', 'group_member_user_deleted');
