-- The chat_project:share enum value cannot be dropped.

-- Sub-chats carry no project_id; deleting only the roots would turn them
-- into top-level chats.
WITH roots AS (
    SELECT chats.id
    FROM chats
    JOIN chat_projects ON chat_projects.id = chats.project_id
    WHERE chat_projects.deleted
)
DELETE FROM chats
WHERE id IN (SELECT id FROM roots)
    OR root_chat_id IN (SELECT id FROM roots);
DELETE FROM chat_projects WHERE deleted;
ALTER TABLE chat_projects DROP COLUMN deleted;

DROP INDEX IF EXISTS idx_chat_projects_group_acl;
DROP INDEX IF EXISTS idx_chat_projects_user_acl;

ALTER TABLE chat_projects
    DROP CONSTRAINT IF EXISTS chat_projects_group_acl_is_object,
    DROP CONSTRAINT IF EXISTS chat_projects_user_acl_is_object,
    DROP COLUMN IF EXISTS group_acl,
    DROP COLUMN IF EXISTS user_acl;
