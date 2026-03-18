-- Best-effort reversal for development use only.
-- Lossy: if the UP migration clamped input_tokens to 0 (because
-- input_tokens < cache_read_tokens), the original value is lost;
-- this reversal produces input_tokens = current + cache_read_tokens.
WITH overcharge AS (
    SELECT
        msg.id,
        COALESCE(msg.input_tokens, 0) + COALESCE(msg.cache_read_tokens, 0) AS restored_input_tokens,
        (
            COALESCE(msg.cache_read_tokens, 0)::numeric
            * COALESCE(pricing.input_price, 0)
        )::bigint AS cost_delta
    FROM
        chat_messages AS msg
    JOIN
        chat_model_configs AS cfg
    ON
        cfg.id = msg.model_config_id
    CROSS JOIN LATERAL (
        SELECT
            COALESCE(
                (cfg.options -> 'cost' ->> 'input_price_per_million_tokens')::numeric,
                (cfg.options ->> 'input_price_per_million_tokens')::numeric
            ) AS input_price
    ) AS pricing
    WHERE
        cfg.provider IN ('openai', 'azure')
        AND msg.cache_read_tokens IS NOT NULL
        AND msg.cache_read_tokens > 0
        AND msg.input_tokens IS NOT NULL
)
UPDATE
    chat_messages AS msg
SET
    input_tokens = overcharge.restored_input_tokens,
    total_cost_micros = CASE
        WHEN msg.total_cost_micros IS NOT NULL AND overcharge.cost_delta > 0
        THEN msg.total_cost_micros + overcharge.cost_delta
        ELSE msg.total_cost_micros
    END
FROM
    overcharge
WHERE
    msg.id = overcharge.id;
