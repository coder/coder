DROP INDEX IF EXISTS idx_chats_root_chat_id;
DROP INDEX IF EXISTS idx_chats_parent_chat_id;

ALTER TABLE chats
    DROP COLUMN IF EXISTS task_status,
    DROP COLUMN IF EXISTS task_report,
    DROP COLUMN IF EXISTS root_chat_id,
    DROP COLUMN IF EXISTS parent_chat_id;

DROP TYPE IF EXISTS chat_task_status;
