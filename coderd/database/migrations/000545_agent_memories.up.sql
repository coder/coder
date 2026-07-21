CREATE TABLE agent_memories (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    path text NOT NULL,
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    search_vector tsvector GENERATED ALWAYS AS (
        to_tsvector(
            'simple'::regconfig,
            translate(path, '/._-', '    ') || ' ' || content
        )
    ) STORED,
    CONSTRAINT agent_memories_path_size CHECK (octet_length(path) <= 1024),
    CONSTRAINT agent_memories_content_size CHECK (octet_length(content) <= 65536)
);

CREATE UNIQUE INDEX agent_memories_user_id_path_idx
    ON agent_memories (user_id, path);

CREATE INDEX agent_memories_search_vector_idx
    ON agent_memories USING gin (search_vector);

-- agent_memories.user_id has ON DELETE CASCADE, but user deletion normally
-- soft-deletes the users row, so the FK cascade does not fire.
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

        -- Remove their agent_memories.
        -- agent_memories.user_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM agent_memories
        WHERE user_id = OLD.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION upsert_agent_memory_fail_if_user_deleted() RETURNS trigger
    LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM 1
    FROM users
    WHERE id = NEW.user_id
      AND deleted = true
    LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION 'Cannot create or update agent memory for deleted user'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'agent_memory_user_deleted';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_upsert_agent_memories
    BEFORE INSERT OR UPDATE ON agent_memories
    FOR EACH ROW
EXECUTE PROCEDURE upsert_agent_memory_fail_if_user_deleted();

-- Adds API key scopes for managing agent memories.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'agent_memory:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'agent_memory:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'agent_memory:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'agent_memory:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'agent_memory:*';
