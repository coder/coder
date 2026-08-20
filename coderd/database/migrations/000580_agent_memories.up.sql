-- Creates the user_memories and chat_memories tables for agent memory.
-- User memories are private per-user documents; chat memories belong to a
-- root chat and are shared with its descendant subagent chats.
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

COMMENT ON TABLE user_memories IS 'Private per-user agent memory documents addressed by scope-relative paths.';

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

COMMENT ON TABLE chat_memories IS 'Agent memory documents owned by a root chat and shared with its descendant subagent chats.';

CREATE UNIQUE INDEX chat_memories_root_chat_id_path_idx ON chat_memories (root_chat_id, path);

-- Enforces the per-user memory cap at the schema level so the invariant
-- survives any future refactor of InsertUserMemory.
CREATE FUNCTION enforce_user_memories_per_user_limit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    memory_count int;
    memory_limit constant int := 100;
BEGIN
    -- Serialize memory-cap checks per user so concurrent inserts cannot all
    -- observe the same pre-insert count and exceed the hard limit.
    PERFORM 1
    FROM users
    WHERE id = NEW.user_id
    FOR UPDATE;

    SELECT count(*) INTO memory_count
    FROM user_memories
    WHERE user_id = NEW.user_id;
    IF memory_count >= memory_limit THEN
        RAISE EXCEPTION 'user has reached the memory limit'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_memories_per_user_limit';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_user_memories_per_user_limit
BEFORE INSERT ON user_memories
FOR EACH ROW
EXECUTE PROCEDURE enforce_user_memories_per_user_limit();

-- Enforces the per-root-chat memory cap at the schema level.
CREATE FUNCTION enforce_chat_memories_per_root_chat_limit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    memory_count int;
    memory_limit constant int := 100;
BEGIN
    -- Serialize memory-cap checks per root chat so concurrent inserts cannot
    -- all observe the same pre-insert count and exceed the hard limit.
    PERFORM 1
    FROM chats
    WHERE id = NEW.root_chat_id
    FOR UPDATE;

    SELECT count(*) INTO memory_count
    FROM chat_memories
    WHERE root_chat_id = NEW.root_chat_id;
    IF memory_count >= memory_limit THEN
        RAISE EXCEPTION 'chat has reached the memory limit'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'chat_memories_per_root_chat_limit';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_chat_memories_per_root_chat_limit
BEFORE INSERT ON chat_memories
FOR EACH ROW
EXECUTE PROCEDURE enforce_chat_memories_per_root_chat_limit();

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

-- Prevent adding new user_memories for soft-deleted users.
-- Closes the window between an in-flight memory write and the
-- soft-delete UPDATE committing.
CREATE FUNCTION insert_user_memory_fail_if_user_deleted() RETURNS trigger
    LANGUAGE plpgsql
AS $$

BEGIN
    PERFORM 1
    FROM users
    WHERE id = NEW.user_id
      AND deleted = true
    LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION 'Cannot create user_memory for deleted user'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_memory_user_deleted';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_upsert_user_memories
    BEFORE INSERT OR UPDATE ON user_memories
    FOR EACH ROW
EXECUTE PROCEDURE insert_user_memory_fail_if_user_deleted();

-- Adds the user memory audit resource type.
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'user_memory';

-- Adds API key scopes for managing user memories. Chat memories are
-- authorized through the root chat's existing chat scopes.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_memory:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_memory:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_memory:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_memory:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'user_memory:*';
