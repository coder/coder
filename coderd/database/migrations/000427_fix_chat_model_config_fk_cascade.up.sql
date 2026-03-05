-- Fix FK violations when deleting chat_providers rows.
-- chat_providers.provider cascades to chat_model_configs, but
-- chat_messages.model_config_id and chats.last_model_config_id had no
-- ON DELETE action, causing FK errors. Switch both to ON DELETE SET NULL.

-- 1. chat_messages.model_config_id → ON DELETE SET NULL
ALTER TABLE chat_messages
    DROP CONSTRAINT chat_messages_model_config_id_fkey;

ALTER TABLE chat_messages
    ADD CONSTRAINT chat_messages_model_config_id_fkey
    FOREIGN KEY (model_config_id) REFERENCES chat_model_configs(id)
    ON DELETE SET NULL;

-- 2. chats.last_model_config_id: allow NULL, then switch to ON DELETE SET NULL
ALTER TABLE chats
    ALTER COLUMN last_model_config_id DROP NOT NULL;

ALTER TABLE chats
    DROP CONSTRAINT chats_last_model_config_id_fkey;

ALTER TABLE chats
    ADD CONSTRAINT chats_last_model_config_id_fkey
    FOREIGN KEY (last_model_config_id) REFERENCES chat_model_configs(id)
    ON DELETE SET NULL;
