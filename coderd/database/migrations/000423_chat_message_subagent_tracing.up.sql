ALTER TABLE chat_messages
    ADD COLUMN subagent_request_id UUID,
    ADD COLUMN subagent_event TEXT;

ALTER TABLE chat_messages
    ADD CONSTRAINT chat_messages_subagent_event_check
    CHECK (subagent_event IS NULL OR subagent_event IN ('request', 'response'));

CREATE INDEX idx_chat_messages_subagent_request
    ON chat_messages(chat_id, subagent_request_id, created_at)
    WHERE subagent_request_id IS NOT NULL;
