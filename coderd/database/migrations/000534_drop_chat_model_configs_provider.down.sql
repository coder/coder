ALTER TABLE chat_model_configs ADD COLUMN provider text;

UPDATE chat_model_configs cmc
SET provider = ap.type::text
FROM ai_providers ap
WHERE ap.id = cmc.ai_provider_id;

UPDATE chat_model_configs SET provider = '' WHERE provider IS NULL;

ALTER TABLE chat_model_configs ALTER COLUMN provider SET NOT NULL;

CREATE INDEX idx_chat_model_configs_provider ON chat_model_configs USING btree (provider);
CREATE INDEX idx_chat_model_configs_provider_model ON chat_model_configs USING btree (provider, model);
