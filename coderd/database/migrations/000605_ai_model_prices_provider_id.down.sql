-- Provider-specific prices have no place in the (provider, model, source) key,
-- so they are dropped to restore it.
DELETE FROM ai_model_prices WHERE provider_id IS NOT NULL;

DROP INDEX ai_model_prices_provider_id_model_source_idx;

DROP INDEX ai_model_prices_provider_model_source_idx;

ALTER TABLE ai_model_prices ADD PRIMARY KEY (provider, model, source);

ALTER TABLE ai_model_prices DROP CONSTRAINT ai_model_prices_provider_id_custom_check;

ALTER TABLE ai_model_prices DROP COLUMN provider_id;
