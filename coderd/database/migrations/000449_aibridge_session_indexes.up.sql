-- A "session" groups related interceptions together. See the COMMENT ON
-- COLUMN below for the full business-logic description.
ALTER TABLE aibridge_interceptions
    ADD COLUMN session_id TEXT NOT NULL
        GENERATED ALWAYS AS (
            COALESCE(
                client_session_id,
                thread_root_id::text,
                id::text
            )
        ) STORED;

-- Searching and grouping on the resolved session ID will be common.
CREATE INDEX idx_aibridge_interceptions_session_id
    ON aibridge_interceptions (session_id)
    WHERE ended_at IS NOT NULL;

COMMENT ON COLUMN aibridge_interceptions.session_id IS
    'Groups related interceptions into a logical session. '
    'Determined by a priority chain: '
    '(1) client_session_id — an explicit session identifier supplied by the '
    'calling client (e.g. Claude Code); '
    '(2) thread_root_id — the root of an agentic thread detected by Bridge '
    'through tool-call correlation, used when the client does not supply its '
    'own session ID; '
    '(3) id — the interception''s own ID, used as a last resort so every '
    'interception belongs to exactly one session even if it is standalone. '
    'This is a generated column stored on disk so it can be indexed and '
    'joined without recomputing the COALESCE on every query.';

-- Composite index for the most common filter path used by
-- ListAIBridgeSessions: initiator_id equality + started_at range,
-- with ended_at IS NOT NULL as a partial filter.
CREATE INDEX idx_aibridge_interceptions_sessions_filter
    ON aibridge_interceptions (initiator_id, started_at DESC, id DESC)
    WHERE ended_at IS NOT NULL;

-- Supports lateral prompt lookup by interception + recency.
CREATE INDEX idx_aibridge_user_prompts_interception_created
    ON aibridge_user_prompts (interception_id, created_at DESC, id DESC);
