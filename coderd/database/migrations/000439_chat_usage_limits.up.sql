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

-- 2. Per-user overrides
CREATE TABLE chat_usage_limit_overrides (
    id              BIGSERIAL   PRIMARY KEY,
    user_id         UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    limit_micros    BIGINT      NOT NULL DEFAULT 0
                    CHECK (limit_micros >= 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id)
);

-- 3. Per-group overrides
CREATE TABLE chat_usage_limit_group_overrides (
    id              BIGSERIAL   PRIMARY KEY,
    group_id        UUID        NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    limit_micros    BIGINT      NOT NULL DEFAULT 0
                    CHECK (limit_micros >= 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id)
);
