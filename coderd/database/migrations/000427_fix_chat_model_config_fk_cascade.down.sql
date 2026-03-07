-- Revert FK cascade changes: restore original constraints without
-- ON DELETE SET NULL and re-add NOT NULL on chats.last_model_config_id.

-- 1. chat_messages.model_config_id → remove ON DELETE SET NULL
ALTER TABLE chat_messages
    DROP CONSTRAINT chat_messages_model_config_id_fkey;

ALTER TABLE chat_messages
    ADD CONSTRAINT chat_messages_model_config_id_fkey
    FOREIGN KEY (model_config_id) REFERENCES chat_model_configs(id);

-- 2. chats.last_model_config_id → remove ON DELETE SET NULL and restore NOT NULL
ALTER TABLE chats
    DROP CONSTRAINT chats_last_model_config_id_fkey;

ALTER TABLE chats
    ADD CONSTRAINT chats_last_model_config_id_fkey
    FOREIGN KEY (last_model_config_id) REFERENCES chat_model_configs(id);

ALTER TABLE chats
    ALTER COLUMN last_model_config_id SET NOT NULL;
