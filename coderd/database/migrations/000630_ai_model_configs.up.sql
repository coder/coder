-- Organization configuration for AI models, for AI Governance. Token prices
-- are the first settings. They override the site-wide ai_model_prices catalog.

-- Configuration for one model on one configured provider within an
-- organization. The provider type is not stored, it comes from ai_providers.
CREATE TABLE ai_model_configs (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider_id       UUID        NOT NULL REFERENCES ai_providers(id) ON DELETE CASCADE,
    model             TEXT        NOT NULL CHECK (model <> ''),
    -- Prices per million tokens, in micro-units (1 unit = 1,000,000).
    -- A NULL column means the price is unknown for this dimension; an explicit zero means "free".
    input_price       BIGINT      CHECK (input_price >= 0),
    output_price      BIGINT      CHECK (output_price >= 0),
    cache_read_price  BIGINT      CHECK (cache_read_price >= 0),
    cache_write_price BIGINT      CHECK (cache_write_price >= 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted           BOOLEAN     NOT NULL DEFAULT FALSE
);

COMMENT ON TABLE ai_model_configs IS 'AI Governance configuration for one model on one configured AI provider within one organization. The token prices override site-wide prices in ai_model_prices.';

COMMENT ON COLUMN ai_model_configs.deleted IS 'Soft delete flag. Soft-deleted rows are kept for history and do not block a new live row for the same model.';

-- A model has at most one live configuration per organization and provider.
CREATE UNIQUE INDEX ai_model_configs_organization_provider_model_unique
    ON ai_model_configs (organization_id, provider_id, model)
    WHERE deleted = FALSE;

-- Supports the cascade from ai_providers, which the unique index cannot serve.
CREATE INDEX idx_ai_model_configs_provider_id ON ai_model_configs (provider_id);
