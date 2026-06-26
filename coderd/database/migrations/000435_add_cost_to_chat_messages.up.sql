ALTER TABLE chat_messages ADD COLUMN total_cost_micros BIGINT;

WITH message_costs AS (
    SELECT
        msg.id,
        ROUND(
            COALESCE(msg.input_tokens, 0)::numeric * COALESCE(pricing.input_price, 0)
            + COALESCE(msg.output_tokens, 0)::numeric * COALESCE(pricing.output_price, 0)
            + COALESCE(msg.cache_read_tokens, 0)::numeric * COALESCE(pricing.cache_read_price, 0)
            + COALESCE(msg.cache_creation_tokens, 0)::numeric * COALESCE(pricing.cache_write_price, 0)
        )::bigint AS total_cost_micros
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
            ) AS input_price,
            COALESCE(
                (cfg.options -> 'cost' ->> 'output_price_per_million_tokens')::numeric,
                (cfg.options ->> 'output_price_per_million_tokens')::numeric
            ) AS output_price,
            COALESCE(
                (cfg.options -> 'cost' ->> 'cache_read_price_per_million_tokens')::numeric,
                (cfg.options ->> 'cache_read_price_per_million_tokens')::numeric
            ) AS cache_read_price,
            COALESCE(
                (cfg.options -> 'cost' ->> 'cache_write_price_per_million_tokens')::numeric,
                (cfg.options ->> 'cache_write_price_per_million_tokens')::numeric
            ) AS cache_write_price
    ) AS pricing
    WHERE
        msg.total_cost_micros IS NULL
        AND (
            msg.input_tokens IS NOT NULL
            OR msg.output_tokens IS NOT NULL
            OR msg.reasoning_tokens IS NOT NULL
            OR msg.cache_creation_tokens IS NOT NULL
            OR msg.cache_read_tokens IS NOT NULL
        )
        AND (
            pricing.input_price IS NOT NULL
            OR pricing.output_price IS NOT NULL
            OR pricing.cache_read_price IS NOT NULL
            OR pricing.cache_write_price IS NOT NULL
        )
        AND (
            (msg.input_tokens IS NOT NULL AND pricing.input_price IS NOT NULL)
            OR (msg.output_tokens IS NOT NULL AND pricing.output_price IS NOT NULL)
            OR (msg.cache_read_tokens IS NOT NULL AND pricing.cache_read_price IS NOT NULL)
            OR (msg.cache_creation_tokens IS NOT NULL AND pricing.cache_write_price IS NOT NULL)
        )
)
UPDATE
    chat_messages AS msg
SET
    total_cost_micros = message_costs.total_cost_micros
FROM
    message_costs
WHERE
    msg.id = message_costs.id;

CREATE INDEX idx_chat_messages_created_at ON chat_messages (created_at);
