-- Enum additions to resource_type and api_key_scope are intentionally not
-- reverted because Postgres cannot drop enum values safely.
DROP TRIGGER IF EXISTS trigger_upsert_user_memories ON user_memories;
DROP FUNCTION IF EXISTS insert_user_memory_fail_if_user_deleted;

-- Restore the previous body of delete_deleted_user_resources() from
-- migration 000503 (without the user_memories cleanup).
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
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trigger_user_memories_per_user_limit ON user_memories;
DROP FUNCTION IF EXISTS enforce_user_memories_per_user_limit();
DROP TRIGGER IF EXISTS trigger_chat_memories_per_root_chat_limit ON chat_memories;
DROP FUNCTION IF EXISTS enforce_chat_memories_per_root_chat_limit();
DROP TABLE user_memories;
DROP TABLE chat_memories;
