-- 1. Singleton config table
CREATE TABLE chat_usage_limit_config (
    id              BIGSERIAL   PRIMARY KEY,
    -- Only one row allowed (enforced by CHECK).
    singleton       BOOLEAN     NOT NULL DEFAULT TRUE CHECK (singleton),
    UNIQUE (singleton),
    enabled         BOOLEAN     NOT NULL DEFAULT FALSE,
    -- Limit per user per period, in micro-dollars (1 USD = 1,000,000).
    default_limit_micros BIGINT NOT NULL DEFAULT 0
                         CHECK (default_limit_micros >= 0),
    -- Period length: 'day', 'week', or 'month'.
    period          TEXT        NOT NULL DEFAULT 'month'
                    CHECK (period IN ('day', 'week', 'month')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed a single disabled row so reads never return empty.
INSERT INTO chat_usage_limit_config (singleton) VALUES (TRUE);

-- 2. Per-user overrides (inline on users table).
ALTER TABLE users ADD COLUMN chat_spend_limit_micros BIGINT DEFAULT NULL
    CHECK (chat_spend_limit_micros IS NULL OR chat_spend_limit_micros > 0);

-- 3. Per-group overrides (inline on groups table).
ALTER TABLE groups ADD COLUMN chat_spend_limit_micros BIGINT DEFAULT NULL
    CHECK (chat_spend_limit_micros IS NULL OR chat_spend_limit_micros > 0);

-- Speed up per-user spend aggregation in the usage-limit hot path.
CREATE INDEX idx_chat_messages_owner_spend
    ON chat_messages (chat_id, created_at)
    WHERE total_cost_micros IS NOT NULL;
