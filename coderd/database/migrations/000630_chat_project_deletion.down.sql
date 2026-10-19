DROP INDEX IF EXISTS idx_chats_project_id_unarchived;

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
