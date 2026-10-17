ALTER TABLE ai_model_prices
    ADD COLUMN tiers jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD CONSTRAINT ai_model_prices_tiers_check CHECK (jsonb_typeof(tiers) = 'array');

COMMENT ON COLUMN ai_model_prices.tiers IS 'Long-context prices as a JSON array sorted by threshold_tokens. A tier applies when the prompt of a request is over its threshold_tokens; a missing tier price uses the base price.';

ALTER TABLE aibridge_token_usages
    ADD COLUMN prompt_tokens bigint,
    ADD COLUMN price_tier_threshold_tokens bigint;

COMMENT ON COLUMN aibridge_token_usages.prompt_tokens IS 'The prompt size of the upstream request, which selects the long-context price tier. NULL on rows recorded before it was tracked.';
COMMENT ON COLUMN aibridge_token_usages.price_tier_threshold_tokens IS 'The threshold_tokens of the long-context price tier that applied. NULL when the base prices applied.';
