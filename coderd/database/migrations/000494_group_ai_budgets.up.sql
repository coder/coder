CREATE TABLE group_ai_budgets (
    group_id    UUID          PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    -- Per-user spend limit, in micro-units (1 unit = 1,000,000).
    spend_limit_micros BIGINT NOT NULL CHECK (spend_limit_micros > 0),
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE group_ai_budgets IS 'Per-group AI spend limit applied to each member of the group. No row means no budget is enforced.';
