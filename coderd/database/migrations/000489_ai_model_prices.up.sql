CREATE TABLE ai_model_prices (
    provider          TEXT        NOT NULL,
    model             TEXT        NOT NULL,
    -- Prices per million tokens, in micro-units (1 unit = 1,000,000).
    -- A NULL column means the price is unknown for this dimension; an explicit zero means "free".
    input_price       BIGINT      CHECK (input_price >= 0),
    output_price      BIGINT      CHECK (output_price >= 0),
    cache_read_price  BIGINT      CHECK (cache_read_price >= 0),
    cache_write_price BIGINT      CHECK (cache_write_price >= 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (provider, model)
);

COMMENT ON TABLE ai_model_prices IS 'Per-model token prices used by AI Bridge to compute interception cost.';

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_model_price:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_model_price:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_model_price:update';
