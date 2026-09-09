-- An application inference profile ARN identifies a Bedrock billing wrapper
-- rather than a model, and the model it wraps is fixed: Bedrock offers no way
-- to repoint a profile, so a new target requires a new profile and a new ARN.
--
-- This table records what each ARN resolves to, so the gateway can detect
-- capabilities, price usage, and record interceptions without calling the
-- Bedrock control plane. Rows are written when a provider is saved and are
-- never invalidated, only corrected by a later save.
CREATE TABLE ai_bedrock_inference_profile_models (
	inference_profile_arn text PRIMARY KEY,
	resolved_model text NOT NULL
);

COMMENT ON COLUMN ai_bedrock_inference_profile_models.resolved_model IS 'The Bedrock model ID the inference profile wraps.';
