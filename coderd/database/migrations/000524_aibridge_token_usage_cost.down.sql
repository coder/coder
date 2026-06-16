ALTER TABLE aibridge_token_usages
    DROP COLUMN effective_group_id,
    DROP COLUMN input_price_micros,
    DROP COLUMN output_price_micros,
    DROP COLUMN cache_read_price_micros,
    DROP COLUMN cache_write_price_micros,
    DROP COLUMN cost_micros;
