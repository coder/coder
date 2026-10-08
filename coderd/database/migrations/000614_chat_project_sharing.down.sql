-- The chat_project:share enum value cannot be dropped.

DELETE FROM chat_projects WHERE deleted;
ALTER TABLE chat_projects DROP COLUMN deleted;

ALTER TABLE chats
    DROP CONSTRAINT chats_project_id_fkey,
    ADD CONSTRAINT chats_project_id_fkey FOREIGN KEY (project_id) REFERENCES chat_projects(id) ON DELETE SET NULL;

DROP INDEX IF EXISTS idx_chat_projects_group_acl;
DROP INDEX IF EXISTS idx_chat_projects_user_acl;

ALTER TABLE chat_projects
    DROP CONSTRAINT IF EXISTS chat_projects_group_acl_is_object,
    DROP CONSTRAINT IF EXISTS chat_projects_user_acl_is_object,
    DROP COLUMN IF EXISTS group_acl,
    DROP COLUMN IF EXISTS user_acl;
