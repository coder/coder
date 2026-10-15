ALTER TABLE aibridge_token_usages
    DROP CONSTRAINT aibridge_token_usages_cache_write_1h_price_micros_check,
    DROP CONSTRAINT aibridge_token_usages_cache_write_1h_input_tokens_check,
    DROP COLUMN price_tier_threshold_tokens,
    DROP COLUMN cache_write_1h_price_micros,
    DROP COLUMN prompt_tokens,
    DROP COLUMN cache_write_1h_input_tokens;

ALTER TABLE ai_model_prices
    DROP CONSTRAINT ai_model_prices_tiers_check,
    DROP CONSTRAINT ai_model_prices_cache_write_1h_price_check,
    DROP COLUMN tiers,
    DROP COLUMN cache_write_1h_price;
