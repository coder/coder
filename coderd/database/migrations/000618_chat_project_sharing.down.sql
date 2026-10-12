-- The chat_project:share enum value cannot be dropped.

-- Sub-chats carry their root's project_id, so this deletes whole families.
DELETE FROM chats
USING chat_projects
WHERE chats.project_id = chat_projects.id AND chat_projects.deleted;
DELETE FROM chat_projects WHERE deleted;

COMMENT ON COLUMN chats.project_id IS 'Optional project that groups a root chat with related chats.';
DROP TRIGGER IF EXISTS trigger_set_sub_chat_project_id ON chats;
DROP FUNCTION IF EXISTS set_sub_chat_project_id();
UPDATE chats SET project_id = NULL WHERE root_chat_id IS NOT NULL OR parent_chat_id IS NOT NULL;
ALTER TABLE chat_projects DROP COLUMN deleted;

DROP INDEX IF EXISTS idx_chat_projects_group_acl;
DROP INDEX IF EXISTS idx_chat_projects_user_acl;

ALTER TABLE chat_projects
    DROP CONSTRAINT IF EXISTS chat_projects_group_acl_is_object,
    DROP CONSTRAINT IF EXISTS chat_projects_user_acl_is_object,
    DROP COLUMN IF EXISTS group_acl,
    DROP COLUMN IF EXISTS user_acl;
