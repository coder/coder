-- Organization price overlay on the site-wide ai_model_prices catalog.

-- A price for one model on one configured provider within an organization.
-- The provider type is not stored, it comes from ai_providers.
CREATE TABLE ai_org_provider_model_prices (
    organization_id   UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider_id       UUID        NOT NULL REFERENCES ai_providers(id) ON DELETE CASCADE,
    model             TEXT        NOT NULL,
    -- Prices per million tokens, in micro-units (1 unit = 1,000,000).
    -- A NULL column means the price is unknown for this dimension; an explicit zero means "free".
    input_price       BIGINT      CHECK (input_price >= 0),
    output_price      BIGINT      CHECK (output_price >= 0),
    cache_read_price  BIGINT      CHECK (cache_read_price >= 0),
    cache_write_price BIGINT      CHECK (cache_write_price >= 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, provider_id, model)
);

COMMENT ON TABLE ai_org_provider_model_prices IS 'Per-model token prices for one configured AI provider within one organization. Overrides site-wide prices in ai_model_prices.';

-- Supports the cascade from ai_providers, which the primary key cannot serve.
CREATE INDEX idx_ai_org_provider_model_prices_provider_id ON ai_org_provider_model_prices (provider_id);
