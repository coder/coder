ALTER TABLE ai_model_prices
    ADD COLUMN cache_write_1h_price bigint,
    ADD CONSTRAINT ai_model_prices_cache_write_1h_price_check CHECK (cache_write_1h_price >= 0);

COMMENT ON COLUMN ai_model_prices.cache_write_1h_price IS 'Price per million tokens of a cache write with a 1-hour TTL. NULL when a 1-hour write costs the cache write price.';
COMMENT ON COLUMN ai_model_prices.tiers IS 'Long-context prices as a JSON array sorted by threshold_tokens. A tier applies when the prompt of a request is over its threshold_tokens; a missing tier price uses the base price, except a missing cache_write_1h_price, which uses the cache write price that applies to the tier.';

ALTER TABLE aibridge_token_usages
    ADD COLUMN cache_write_1h_input_tokens bigint NOT NULL DEFAULT 0,
    ADD COLUMN cache_write_1h_price_micros bigint,
    ADD CONSTRAINT aibridge_token_usages_cache_write_1h_input_tokens_check
        CHECK (cache_write_1h_input_tokens BETWEEN 0 AND cache_write_input_tokens),
    ADD CONSTRAINT aibridge_token_usages_cache_write_1h_price_micros_check CHECK (cache_write_1h_price_micros >= 0);

COMMENT ON COLUMN aibridge_token_usages.cache_write_1h_input_tokens IS 'The part of cache_write_input_tokens written with a 1-hour TTL.';
COMMENT ON COLUMN aibridge_token_usages.cache_write_1h_price_micros IS 'The price per million tokens applied to 1-hour cache writes.';
