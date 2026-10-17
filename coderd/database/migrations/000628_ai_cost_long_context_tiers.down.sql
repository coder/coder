ALTER TABLE aibridge_token_usages
    DROP COLUMN price_tier_threshold_tokens,
    DROP COLUMN prompt_tokens;

ALTER TABLE ai_model_prices
    DROP CONSTRAINT ai_model_prices_tiers_check,
    DROP COLUMN tiers;
