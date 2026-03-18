-- Fix OpenAI/Azure cached-input double-counting.
-- input_tokens included cached tokens; subtract cache_read_tokens
-- so persisted input_tokens represents only non-cached input.
-- For total_cost_micros, subtract only the overcharged portion
-- (cache_read_tokens billed at input_price) rather than
-- recomputing the full cost from current pricing.
WITH overcharge AS (
    SELECT
        msg.id,
        GREATEST(COALESCE(msg.input_tokens, 0) - COALESCE(msg.cache_read_tokens, 0), 0) AS corrected_input_tokens,
        -- The overcharge equals the tokens actually removed from
        -- input_tokens multiplied by the input rate. When cached tokens
        -- exceed input_tokens, clamp the removed-token count so we do
        -- not subtract more cost than was originally charged.
        -- Truncate (floor) rather than ceil to avoid over-subtracting
        -- from the already-rounded total_cost_micros.
        (
            LEAST(COALESCE(msg.input_tokens, 0), COALESCE(msg.cache_read_tokens, 0))::numeric
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
    input_tokens = overcharge.corrected_input_tokens,
    total_cost_micros = CASE
        WHEN msg.total_cost_micros IS NOT NULL AND overcharge.cost_delta > 0
        THEN GREATEST(msg.total_cost_micros - overcharge.cost_delta, 0)
        ELSE msg.total_cost_micros
    END
FROM
    overcharge
WHERE
    msg.id = overcharge.id;
