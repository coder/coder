UPDATE chat_messages
SET api_key_id = NULL
WHERE api_key_id IS NOT NULL
    AND NOT EXISTS (
        SELECT 1
        FROM api_keys
        WHERE api_keys.id = chat_messages.api_key_id
    );

UPDATE chat_queued_messages
SET api_key_id = NULL
WHERE api_key_id IS NOT NULL
    AND NOT EXISTS (
        SELECT 1
        FROM api_keys
        WHERE api_keys.id = chat_queued_messages.api_key_id
    );

ALTER TABLE chat_messages
ADD CONSTRAINT chat_messages_api_key_id_fkey
FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE SET NULL;

ALTER TABLE chat_queued_messages
ADD CONSTRAINT chat_queued_messages_api_key_id_fkey
FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE SET NULL;
