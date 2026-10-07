-- The chat_project:share enum value cannot be dropped.

DROP INDEX IF EXISTS idx_chat_projects_group_acl;
DROP INDEX IF EXISTS idx_chat_projects_user_acl;

ALTER TABLE chat_projects
    DROP CONSTRAINT IF EXISTS chat_projects_group_acl_is_object,
    DROP CONSTRAINT IF EXISTS chat_projects_user_acl_is_object,
    DROP COLUMN IF EXISTS group_acl,
    DROP COLUMN IF EXISTS user_acl;
