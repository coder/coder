-- Creates the user_memories and chat_memories tables for agent memory.
-- User memories are private per-user documents; chat memories belong to a
-- root chat, and access to them is gated on the root chat permissions.
-- Size, count, and path limits must stay in sync with the constants in
-- coderd/x/memory.
--
-- Paths are ASCII-only and case-sensitive by design (matching mux memory
-- semantics): Notes.md and notes.md are distinct documents, and non-ASCII
-- names such as notes/café.md are rejected by the format check.
CREATE TABLE user_memories (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    path text NOT NULL,
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_memories_path_size CHECK (octet_length(path) <= 256),
    CONSTRAINT user_memories_path_format CHECK (
        path ~ '^[a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)*\.md$'
        AND path !~ '(^|/)[.]{1,2}(/|$)'
    ),
    CONSTRAINT user_memories_content_size CHECK (octet_length(content) <= 65536)
);

COMMENT ON TABLE user_memories IS 'Private per-user agent memory documents addressed by scope-relative paths. The owner role retains administrative read and delete access; audit logging treats content as a secret.';

CREATE UNIQUE INDEX user_memories_user_id_path_idx ON user_memories (user_id, path);

CREATE TABLE chat_memories (
    id uuid PRIMARY KEY,
    root_chat_id uuid NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    path text NOT NULL,
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_memories_path_size CHECK (octet_length(path) <= 256),
    CONSTRAINT chat_memories_path_format CHECK (
        path ~ '^[a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)*\.md$'
        AND path !~ '(^|/)[.]{1,2}(/|$)'
    ),
    CONSTRAINT chat_memories_content_size CHECK (octet_length(content) <= 65536)
);

COMMENT ON TABLE chat_memories IS 'Agent memory documents owned by a root chat; access is gated on the root chat permissions. Rows follow the parent chat lifecycle: retention purges of old chats cascade-delete them without per-memory audit events.';

CREATE UNIQUE INDEX chat_memories_root_chat_id_path_idx ON chat_memories (root_chat_id, path);

-- user_memories joins the delete_deleted_user_resources cleanup set below,
-- so it attaches the shared fail_if_user_deleted guard from migration
-- 000591 like the other guarded tables: one encoding of the users-row lock
-- (check_user_not_deleted) and the operation gate. The trigger is
-- INSERT-only rather than also covering UPDATE OF user_id because the
-- owner column is immutable below; there is no reassignment path to guard.
--
-- There is no fail-closed missing-parent branch: user_memories.user_id has
-- a hard foreign key to users, so an absent parent is rejected by the RI
-- check at end of statement. The residual window (a users row invisible to
-- the trigger's statement snapshot but committed before the RI check, which
-- would let the insert pass the guard without the users-row lock) is
-- unreachable through the Store: every caller resolves the user from a
-- committed row before inserting, and at READ COMMITTED the insert
-- statement's snapshot is taken after that read.
--
-- The guard's users-row lock (INSERT path) serializes the cap count below
-- against user soft deletion and concurrent inserts without conflicting
-- with foreign key validation on other child tables. It does conflict with
-- ordinary UPDATE statements on the same users row (for example
-- UpdateUserLastSeenAt, written roughly once per minute per active session
-- by the API key middleware); both sides are single short statements, so
-- the cost is accepted.
--
-- The lock also imposes the ordering contract documented on
-- check_user_not_deleted (migration 000591): a transaction that writes any
-- row that delete_deleted_user_resources deletes (api_keys, user_links,
-- user_secrets, user_skills, user_ai_provider_keys, organization_members,
-- group_members, user_ai_budget_overrides, or a user_memories row) and
-- then inserts a memory for the same user must call
-- AcquireUserSoftDeleteGuardLock first, or it inverts the lock order
-- against that cleanup and deadlocks with a concurrent soft-delete (40P01,
-- which coderd does not retry). The query is dbauthz-authorized as a
-- system primitive: user-scoped callers wrap only that call in
-- dbauthz.AsSystemRestricted.
CREATE TRIGGER trigger_insert_user_memories
    BEFORE INSERT ON user_memories
    FOR EACH ROW
EXECUTE FUNCTION fail_if_user_deleted('user_memory', 'user_memory_user_deleted');

-- Isolation gate for the memory caps only. The caps below count committed
-- sibling rows after a parent-row lock wait, which is race-free exactly at
-- READ COMMITTED: a REPEATABLE READ or SERIALIZABLE snapshot survives the
-- wait and can miss concurrently committed rows, silently overshooting the
-- cap. The gate is deliberately scoped to the two brand-new memory tables:
-- rejecting a stronger level here cannot break any existing feature write
-- (the pre-existing cap triggers state this contract in migration 000590
-- instead of enforcing it, because a runtime gate would turn a
-- deployment-level default_transaction_isolation setting into an outage of
-- shipped features). No production writer of the memory tables runs above
-- READ COMMITTED; do not wrap memory inserts in database.ReadModifyUpdate.
CREATE FUNCTION require_read_committed(guard_name text, constraint_name text) RETURNS void
	LANGUAGE plpgsql
AS $$
BEGIN
	IF current_setting('transaction_isolation') NOT IN ('read committed', 'read uncommitted') THEN
		RAISE EXCEPTION '% requires READ COMMITTED isolation, ran under %',
			guard_name, current_setting('transaction_isolation')
			USING ERRCODE = 'check_violation',
				  CONSTRAINT = constraint_name,
				  DETAIL = 'A stronger level''s snapshot can survive a lock wait and miss concurrently committed rows, overshooting the cap. If no caller set this level, check default_transaction_isolation on the server, database, role, or pooler.';
	END IF;
END;
$$;

-- The per-user cap. Counting committed rows is only correct under READ
-- COMMITTED (see require_read_committed above; the raised error is
-- check_violation, not a serialization failure, so it surfaces to the
-- caller instead of entering any retry loop). The count runs under the
-- users-row lock the guard trigger above already took: BEFORE triggers
-- fire in name order and the zz_ prefix pins guard-before-cap, so no
-- separate advisory lock is needed.
CREATE FUNCTION enforce_user_memories_per_user_limit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    memory_count int;
    memory_limit constant int := 100;
BEGIN
    PERFORM require_read_committed('user_memories cap', 'user_memory_insert_isolation');

    SELECT count(*) INTO memory_count
    FROM user_memories
    WHERE user_id = NEW.user_id;
    IF memory_count >= memory_limit THEN
        RAISE EXCEPTION 'user % has reached the user_memories limit of % (current count %)',
            NEW.user_id, memory_limit, memory_count
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_memories_per_user_limit';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_zz_user_memories_per_user_limit
BEFORE INSERT ON user_memories
FOR EACH ROW
EXECUTE PROCEDURE enforce_user_memories_per_user_limit();

-- The insert-path triggers (the shared guard and the cap) fire BEFORE
-- INSERT only; an UPDATE that reassigns the owner column would bypass both
-- (the guard's UPDATE branch checks deleted but the cap never recounts).
-- The owner column is immutable instead, with no parent lock so the UPDATE
-- path cannot deadlock against the soft-delete cleanup.
CREATE FUNCTION enforce_user_memories_owner_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'user_memories.user_id is immutable'
        USING ERRCODE = 'check_violation',
              CONSTRAINT = 'user_memory_owner_immutable';
END;
$$;

-- The WHEN clause (IS DISTINCT FROM, so a NULL assignment is also caught
-- here rather than by the NOT NULL constraint) skips the plpgsql call for
-- ordinary content edits and renames, and makes the condition visible in
-- \d user_memories and dump.sql.
CREATE TRIGGER trigger_user_memories_owner_immutable
BEFORE UPDATE ON user_memories
FOR EACH ROW
WHEN (NEW.user_id IS DISTINCT FROM OLD.user_id)
EXECUTE PROCEDURE enforce_user_memories_owner_immutable();

-- Locking the chat row serializes the count check without conflicting with
-- foreign key validation on other child tables. The lock does contend with
-- ordinary UPDATE statements on the same chats row (title changes, archived flips,
-- heartbeat writes); that cost is accepted because memory inserts are
-- low-frequency and capped.
--
-- The count cap is only race-free under READ COMMITTED (see
-- require_read_committed above); the raised error is a check_violation,
-- not a serialization failure, so it surfaces to the caller instead of
-- entering any retry loop.
--
-- The lock also imposes an ordering contract on callers: a transaction
-- that holds a lock on any chat-owned child row and then inserts a memory
-- for the same root chat inverts the lock order against hard chat deletion
-- (child tuple first, chats second; the retention purge cascade takes
-- chats first) and deadlocks (40P01, which coderd does not retry). Take
-- the chats row lock first: GetChatByIDForUpdate, or ChatMachine.Update,
-- which opens with LockChatAndBumpSnapshotVersion (LockChatByID is
-- system-scoped and not callable as the user). Or do not mix the insert
-- with prior child-row writes in one transaction.
CREATE FUNCTION enforce_chat_memories_insert_invariants() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    referenced_parent_chat_id uuid;
    referenced_root_chat_id uuid;
    memory_count int;
    memory_limit constant int := 100;
BEGIN
    PERFORM require_read_committed('chat_memories cap', 'chat_memory_insert_isolation');

    SELECT parent_chat_id, root_chat_id
    INTO referenced_parent_chat_id, referenced_root_chat_id
    FROM chats
    WHERE id = NEW.root_chat_id
    FOR NO KEY UPDATE;

    -- Fail closed: this trigger reads at its own snapshot while the RI
    -- check re-reads with a fresh snapshot at end of statement, so a chat
    -- committing between the two reads is visible only to the FK check.
    -- Returning NEW here would let a memory land under an unvalidated,
    -- possibly subagent, chat.
    IF NOT FOUND THEN
        RAISE EXCEPTION 'chat % does not exist', NEW.root_chat_id
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'chat_memory_root_chat_required';
    END IF;

    -- chats.parent_chat_id / chats.root_chat_id are immutable through the
    -- Store, but their ON DELETE SET NULL FKs can flip them to NULL when an
    -- ancestor chat is hard-deleted by retention purges. Chat memories under
    -- a purged root are cascade-deleted with it, so no memory row survives
    -- into a promoted subagent namespace; a subagent chat that outlives its
    -- purged root does get both columns nulled and passes this check from
    -- then on, becoming a root chat with a fresh, initially empty namespace.
    -- Both columns are checked because they can be nulled independently:
    -- purging an intermediate subagent chat leaves its children with
    -- parent_chat_id NULL while root_chat_id still points at the real root.
    IF referenced_parent_chat_id IS NOT NULL OR referenced_root_chat_id IS NOT NULL THEN
        RAISE EXCEPTION 'chat % is not a root chat', NEW.root_chat_id
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'chat_memory_root_chat_required';
    END IF;

    SELECT count(*) INTO memory_count
    FROM chat_memories
    WHERE root_chat_id = NEW.root_chat_id;
    IF memory_count >= memory_limit THEN
        RAISE EXCEPTION 'chat % has reached the chat_memories limit of % (current count %)',
            NEW.root_chat_id, memory_limit, memory_count
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'chat_memories_per_root_chat_limit';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_chat_memories_insert_invariants
BEFORE INSERT ON chat_memories
FOR EACH ROW
EXECUTE PROCEDURE enforce_chat_memories_insert_invariants();

-- The insert invariants fire BEFORE INSERT only; an UPDATE that reassigns
-- the owner column would bypass all of them (root-chat and cap checks).
-- The owner column is immutable instead, with no parent lock so the UPDATE
-- path cannot deadlock against hard chat deletion.
CREATE FUNCTION enforce_chat_memories_owner_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'chat_memories.root_chat_id is immutable'
        USING ERRCODE = 'check_violation',
              CONSTRAINT = 'chat_memory_owner_immutable';
END;
$$;

-- See trigger_user_memories_owner_immutable for the WHEN clause rationale.
CREATE TRIGGER trigger_chat_memories_owner_immutable
BEFORE UPDATE ON chat_memories
FOR EACH ROW
WHEN (NEW.root_chat_id IS DISTINCT FROM OLD.root_chat_id)
EXECUTE PROCEDURE enforce_chat_memories_owner_immutable();

-- Extend the soft-delete cleanup trigger to also wipe user_memories.
-- user_memories.user_id has ON DELETE CASCADE, but Coder soft-deletes
-- users by flipping users.deleted instead of removing the row, so the
-- FK cascade never fires and memories would otherwise survive deletion.
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

        -- Remove their user AI provider keys.
        -- user_ai_provider_keys.user_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM user_ai_provider_keys
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

        -- Remove their user_memories.
        -- user_memories.user_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM user_memories
        WHERE user_id = OLD.id;
    END IF;
    RETURN NEW;
END;
$$;

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'user_memory';
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'chat_memory';

-- No chat_memory API key scopes are added; chat memories are authorized
-- through the root chat's existing chat scopes.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_memory:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_memory:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_memory:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_memory:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_memory:*';
