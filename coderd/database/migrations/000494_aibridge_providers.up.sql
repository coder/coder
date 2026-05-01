CREATE TYPE ai_provider_type AS ENUM (
    'openai',
    'anthropic'
);

CREATE TABLE ai_providers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type            ai_provider_type NOT NULL,
    name            text NOT NULL UNIQUE
                        CONSTRAINT ai_providers_name_check
                        CHECK (name ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    display_name    text NOT NULL DEFAULT '',
    enabled         boolean NOT NULL DEFAULT TRUE,
    deleted         boolean NOT NULL DEFAULT FALSE,
    base_url        text NOT NULL,
    api_key         text NOT NULL DEFAULT '',
    api_key_key_id  text REFERENCES dbcrypt_keys(active_key_digest),
    settings        text NOT NULL DEFAULT '',
    settings_key_id text REFERENCES dbcrypt_keys(active_key_digest),
    created_at      timestamp with time zone NOT NULL DEFAULT NOW(),
    updated_at      timestamp with time zone NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE ai_providers IS 'Runtime configuration for AI Bridge providers. Authoritative source for the provider set served by aibridged. Replaces deployment-time CODER_AIBRIDGE_* environment variables.';

COMMENT ON COLUMN ai_providers.api_key IS 'Centralized API key used to authenticate with the upstream AI provider. Encrypted at rest via dbcrypt when api_key_key_id is set.';

COMMENT ON COLUMN ai_providers.api_key_key_id IS 'The ID of the key used to encrypt the provider API key. If this is NULL, the API key is not encrypted.';

COMMENT ON COLUMN ai_providers.settings IS 'Encrypted JSON blob holding type-specific configuration (e.g. AWS Bedrock region, model). Plaintext is a JSON object. Empty string when no type-specific settings are required.';

COMMENT ON COLUMN ai_providers.settings_key_id IS 'The ID of the key used to encrypt settings. If this is NULL, settings is not encrypted.';

COMMENT ON COLUMN ai_providers.deleted IS 'Soft delete flag. Soft-deleted rows are preserved for audit and FK history; their names remain reserved.';

CREATE INDEX idx_ai_providers_enabled ON ai_providers (enabled) WHERE deleted = FALSE;

-- Audit support: allow ai_providers to appear in audit_log.resource_type.
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'ai_provider';

-- API key scopes for ai_provider resources.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'aibridge_provider:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'aibridge_provider:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'aibridge_provider:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'aibridge_provider:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'aibridge_provider:update';
