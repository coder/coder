-- provider_id keys a custom price to one configured provider, so providers of
-- the same type can price the same model differently. Rows without it price
-- every provider of their type and act as the fallback.
-- provider_id is nullable, so it cannot join the primary key. Each kind of row
-- gets its own partial unique index instead.

ALTER TABLE ai_model_prices ADD COLUMN provider_id uuid REFERENCES ai_providers(id) ON DELETE CASCADE;

ALTER TABLE ai_model_prices ADD CONSTRAINT ai_model_prices_provider_id_custom_check
	CHECK (provider_id IS NULL OR source = 'custom');

ALTER TABLE ai_model_prices DROP CONSTRAINT ai_model_prices_pkey;

CREATE UNIQUE INDEX ai_model_prices_provider_model_source_idx
	ON ai_model_prices (provider, model, source)
	WHERE provider_id IS NULL;

CREATE UNIQUE INDEX ai_model_prices_provider_id_model_source_idx
	ON ai_model_prices (provider_id, model, source)
	WHERE provider_id IS NOT NULL;

COMMENT ON COLUMN ai_model_prices.provider_id IS 'The configured provider a custom price applies to. NULL prices every provider of the given provider type. A provider-specific price takes precedence over a provider-type price for the same model.';
