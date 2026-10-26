-- The chat_artifact:share enum value cannot be dropped.

DROP TRIGGER IF EXISTS trigger_delete_chat_artifact_user_acl_on_org_member_delete ON organization_members;
DROP FUNCTION IF EXISTS delete_chat_artifact_user_acl_on_org_member_delete();

DROP INDEX IF EXISTS idx_chat_artifacts_group_acl;
DROP INDEX IF EXISTS idx_chat_artifacts_user_acl;

ALTER TABLE chat_artifacts
    DROP CONSTRAINT IF EXISTS chat_artifacts_group_acl_is_object,
    DROP CONSTRAINT IF EXISTS chat_artifacts_user_acl_is_object,
    DROP COLUMN IF EXISTS group_acl,
    DROP COLUMN IF EXISTS user_acl;

COMMENT ON TABLE chat_artifacts IS 'Versioned files that chats publish. An artifact outlives its chats. The owner sees it, and the members of project_id see it too.';
