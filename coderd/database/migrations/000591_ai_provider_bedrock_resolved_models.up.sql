-- Application inference profile ARNs are opaque: they identify a Bedrock
-- billing wrapper, not a model. This table records the model each configured
-- ARN resolves to, so the gateway can detect capabilities, price usage, and
-- record interceptions without calling the Bedrock control plane itself.
--
-- A provider has a row only while one of its identifiers is an ARN. Providers
-- configured with plain model IDs need no mapping, and neither does a provider
-- whose profile could not be resolved: the gateway refuses to serve it.
CREATE TABLE ai_provider_bedrock_resolved_models (
	ai_provider_id uuid PRIMARY KEY REFERENCES ai_providers (id) ON DELETE CASCADE,
	resolved_model text NOT NULL,
	resolved_small_fast_model text NOT NULL
);

COMMENT ON COLUMN ai_provider_bedrock_resolved_models.resolved_model IS 'The model ID behind the provider''s configured model identifier. Equal to the configured value when that value is already a model ID.';

COMMENT ON COLUMN ai_provider_bedrock_resolved_models.resolved_small_fast_model IS 'resolved_model for the provider''s configured small/fast model identifier.';
