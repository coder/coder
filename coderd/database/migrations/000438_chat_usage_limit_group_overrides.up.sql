CREATE TABLE chat_usage_limit_group_overrides (
    id              BIGSERIAL   PRIMARY KEY,
    group_id        UUID        NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    limit_micros    BIGINT      NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id)
);
