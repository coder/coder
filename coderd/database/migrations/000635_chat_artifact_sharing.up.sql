ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_artifact:share';

ALTER TABLE chat_artifacts
    ADD COLUMN user_acl jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN group_acl jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE chat_artifacts
    ADD CONSTRAINT chat_artifacts_user_acl_is_object
        CHECK (jsonb_typeof(user_acl) = 'object'),
    ADD CONSTRAINT chat_artifacts_group_acl_is_object
        CHECK (jsonb_typeof(group_acl) = 'object');

COMMENT ON TABLE chat_artifacts IS 'Versioned files that chats publish. An artifact outlives its chats. The owner sees it, the members of project_id see it, and the users and groups in user_acl and group_acl can read it.';
COMMENT ON COLUMN chat_artifacts.user_acl IS 'Users the artifact is shared with for reading, keyed by user ID.';
COMMENT ON COLUMN chat_artifacts.group_acl IS 'Groups the artifact is shared with for reading, keyed by group ID. The organization ID is the Everyone group.';

CREATE INDEX idx_chat_artifacts_user_acl ON chat_artifacts USING gin (user_acl);
CREATE INDEX idx_chat_artifacts_group_acl ON chat_artifacts USING gin (group_acl);

CREATE FUNCTION delete_chat_artifact_user_acl_on_org_member_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    -- Drop the departing member's direct grants so adding them back to the
    -- organization does not silently restore access.
    UPDATE chat_artifacts
    SET user_acl = user_acl - OLD.user_id::text
    WHERE organization_id = OLD.organization_id AND user_acl ? OLD.user_id::text;
    RETURN OLD;
END;
$$;

CREATE TRIGGER trigger_delete_chat_artifact_user_acl_on_org_member_delete
    BEFORE DELETE ON organization_members
    FOR EACH ROW
    EXECUTE FUNCTION delete_chat_artifact_user_acl_on_org_member_delete();
