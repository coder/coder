DROP INDEX idx_chat_model_configs_provider;
DROP INDEX idx_chat_model_configs_provider_model;

ALTER TABLE chat_model_configs DROP COLUMN provider;
