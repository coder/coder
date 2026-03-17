-- Composite index for the most common filter path used by
-- ListAIBridgeSessions: initiator_id equality + started_at range,
-- with ended_at IS NOT NULL as a partial filter.
CREATE INDEX idx_aibridge_interceptions_sessions_filter
    ON aibridge_interceptions (initiator_id, started_at DESC, id DESC)
    WHERE ended_at IS NOT NULL;

-- Supports lateral prompt lookup by interception + recency.
CREATE INDEX idx_aibridge_user_prompts_interception_created
    ON aibridge_user_prompts (interception_id, created_at DESC, id DESC);
