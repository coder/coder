ALTER TABLE chat_model_configs
	DROP CONSTRAINT IF EXISTS chat_model_configs_provider_model_key;

CREATE INDEX IF NOT EXISTS idx_chat_model_configs_provider_model
	ON chat_model_configs(provider, model);
