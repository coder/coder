ALTER TABLE ai_model_prices
    ADD COLUMN pricing_extras jsonb,
    ADD CONSTRAINT ai_model_prices_pricing_extras_check CHECK (jsonb_typeof(pricing_extras) = 'object');

COMMENT ON COLUMN ai_model_prices.pricing_extras IS
    'Prices beyond the four base price columns, such as the 1-hour cache write price and long-context tiers, as a JSON object. NULL when the model has none.';

-- The table is large and every existing row is NULL, so the check skips
-- validating existing rows to avoid a full scan under an exclusive lock.
ALTER TABLE aibridge_token_usages
    ADD COLUMN pricing_usage jsonb,
    ADD CONSTRAINT aibridge_token_usages_pricing_usage_check CHECK (jsonb_typeof(pricing_usage) = 'object') NOT VALID;

COMMENT ON COLUMN aibridge_token_usages.pricing_usage IS
    'How the row was billed beyond the four price columns, such as the prompt size, the long-context tier that applied, and extra billed units, as a JSON object. NULL when the row was not priced or was recorded before it was tracked.';
