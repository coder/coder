ALTER TABLE aibridge_token_usages
    -- Effective group this interception's spend is attributed to. NULL if the
    -- user has no effective group (no budget configured). Intentionally not a
    -- foreign key: this is an immutable historical attribution that must
    -- survive group deletion, so the id is retained even after the group is gone.
    ADD COLUMN effective_group_id UUID,
    -- Snapshotted prices at interception time, in micro-units per million
    -- tokens. NULL if the model is not present in ai_model_prices.
    ADD COLUMN input_price_micros       BIGINT CHECK (input_price_micros >= 0),
    ADD COLUMN output_price_micros      BIGINT CHECK (output_price_micros >= 0),
    ADD COLUMN cache_read_price_micros  BIGINT CHECK (cache_read_price_micros >= 0),
    ADD COLUMN cache_write_price_micros BIGINT CHECK (cache_write_price_micros >= 0),
    -- Computed cost in micro-units at interception time. NULL if the model is
    -- not present in ai_model_prices.
    ADD COLUMN cost_micros              BIGINT CHECK (cost_micros >= 0);
