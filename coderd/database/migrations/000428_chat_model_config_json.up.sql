ALTER TABLE chat_model_configs
    ADD COLUMN IF NOT EXISTS model_config JSONB NOT NULL DEFAULT '{}'::jsonb;
