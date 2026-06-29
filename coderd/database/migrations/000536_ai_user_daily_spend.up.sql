-- Aggregates a user's AI spend within their effective group, one row per
-- UTC day. Drives budget enforcement and reporting.
CREATE TABLE ai_user_daily_spend (
    -- No FK to users. Spend records persist after user deletion.
    user_id            UUID   NOT NULL,
    -- No FK to groups. Spend records persist after group deletion.
    effective_group_id UUID   NOT NULL,
    day                DATE   NOT NULL,
    spend_micros       BIGINT NOT NULL CHECK (spend_micros >= 0),
    PRIMARY KEY (user_id, effective_group_id, day)
);

COMMENT ON TABLE ai_user_daily_spend IS 'Daily AI spend per user and effective group.';
COMMENT ON COLUMN ai_user_daily_spend.user_id IS 'The user who incurred the spend.';
COMMENT ON COLUMN ai_user_daily_spend.effective_group_id IS 'The group this spend is attributed to for budget purposes.';
COMMENT ON COLUMN ai_user_daily_spend.day IS 'UTC calendar day the spend was incurred.';
COMMENT ON COLUMN ai_user_daily_spend.spend_micros IS 'Accumulated spend in micro-units (1 unit = 1,000,000).';

-- For queries filtering by effective_group_id alone.
CREATE INDEX idx_ai_user_daily_spend_effective_group_id_day
    ON ai_user_daily_spend (effective_group_id, day);
