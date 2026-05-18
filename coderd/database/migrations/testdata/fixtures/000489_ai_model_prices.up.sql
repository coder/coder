INSERT INTO ai_model_prices (
    provider,
    model,
    input_price,
    output_price,
    cache_read_price,
    cache_write_price
) VALUES
    ('anthropic', 'claude-3-5-sonnet-20241022', 3000000, 15000000, 300000, 3750000),
    ('openai', 'gpt-4o', 2500000, 10000000, 1250000, NULL);
