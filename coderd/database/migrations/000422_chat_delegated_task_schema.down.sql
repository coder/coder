DROP INDEX IF EXISTS idx_chats_root_chat_id;
DROP INDEX IF EXISTS idx_chats_parent_chat_id;

ALTER TABLE chats
    DROP COLUMN IF EXISTS root_chat_id,
    DROP COLUMN IF EXISTS parent_chat_id;
