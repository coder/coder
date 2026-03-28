ALTER TABLE chat_model_configs
    ADD COLUMN provider_config_id uuid REFERENCES chat_providers(id) ON DELETE SET NULL;
