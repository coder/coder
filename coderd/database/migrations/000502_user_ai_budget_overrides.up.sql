CREATE TABLE user_ai_budget_overrides (
    user_id            UUID        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    group_id           UUID        NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    -- Spend limit applied to the user, in micro-units (1 unit = 1,000,000).
    spend_limit_micros BIGINT      NOT NULL CHECK (spend_limit_micros >= 0),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- The user must be a member of the attributed group. When the user is
    -- removed from the group, the override is dropped automatically.
    FOREIGN KEY (user_id, group_id) REFERENCES group_members(user_id, group_id) ON DELETE CASCADE
);

COMMENT ON TABLE user_ai_budget_overrides IS 'Per-user AI spend override that supersedes group budget resolution.';
