ALTER TABLE chat_queued_messages
ADD COLUMN model_config_id uuid;

UPDATE chat_queued_messages AS cqm
SET model_config_id = chats.last_model_config_id
FROM chats
WHERE chats.id = cqm.chat_id
  AND cqm.model_config_id IS NULL;
