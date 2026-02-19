DROP INDEX IF EXISTS idx_chat_messages_subagent_request;

ALTER TABLE chat_messages
    DROP CONSTRAINT IF EXISTS chat_messages_subagent_event_check;

ALTER TABLE chat_messages
    DROP COLUMN IF EXISTS subagent_event,
    DROP COLUMN IF EXISTS subagent_request_id;
