ALTER TABLE aibridge_token_usages
    DROP COLUMN IF EXISTS pricing_usage;

ALTER TABLE ai_model_prices
    DROP COLUMN IF EXISTS pricing_extras;
