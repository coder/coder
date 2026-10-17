ALTER TABLE aibridge_token_usages
    DROP CONSTRAINT aibridge_token_usages_cache_write_1h_price_micros_check,
    DROP CONSTRAINT aibridge_token_usages_cache_write_1h_input_tokens_check,
    DROP COLUMN cache_write_1h_price_micros,
    DROP COLUMN cache_write_1h_input_tokens;

ALTER TABLE ai_model_prices
    DROP CONSTRAINT ai_model_prices_cache_write_1h_price_check,
    DROP COLUMN cache_write_1h_price;

COMMENT ON COLUMN ai_model_prices.tiers IS 'Long-context prices as a JSON array sorted by threshold_tokens. A tier applies when the prompt of a request is over its threshold_tokens; a missing tier price uses the base price.';
