-- Materializes AI Bridge sessions, the logical grouping of interceptions
-- sharing a session_id. Without a table of their own, ordering and filtering
-- the sessions list meant aggregating every interception on each page load, and
-- no index could serve the ordering. Storing the ordering key and the
-- filterable attributes here makes a page an index scan that stops after LIMIT
-- rows.
--
-- The two timestamps are used for time-range filtering and sorting:
--
--   column         | definition                   | used for
--   ---------------+------------------------------+------------------------
--   started_at     | MIN(interception.started_at) | filtering
--   last_active_at | MAX(interception.ended_at)   | ordering, filtering
CREATE TABLE aibridge_sessions (
    session_id text NOT NULL,
    initiator_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    started_at timestamptz NOT NULL,
    -- Ordering key: the most recent session's interception's ended_at.
    last_active_at timestamptz NOT NULL,
    -- Filter attributes belong to interceptions, so they are denormalized here
    -- to keep filtered pages on the index. Sessions genuinely may span several
    -- providers and models, so those are arrays; client is a scalar because a
    -- session_id is issued by a single client.
    providers text[] NOT NULL DEFAULT '{}',
    provider_names text[] NOT NULL DEFAULT '{}',
    models text[] NOT NULL DEFAULT '{}',
    client text NOT NULL DEFAULT 'Unknown',
    -- session_id alone is not unique: it derives from the client-supplied
    -- client_session_id, so two users can present the same value. Keying on
    -- both columns keeps their sessions separate, matching the
    -- GROUP BY session_id, initiator_id the query used before.
    PRIMARY KEY (session_id, initiator_id)
);

COMMENT ON TABLE aibridge_sessions IS 'Materialized view of AI Bridge sessions, maintained by a trigger on aibridge_interceptions. Each row summarizes the interceptions sharing same session_id and initiator.';
COMMENT ON COLUMN aibridge_sessions.started_at IS 'Earliest started_at across the session''s interceptions. Paired with last_active_at so time-range filters can test whether the session overlaps the requested window.';
COMMENT ON COLUMN aibridge_sessions.last_active_at IS 'Latest ended_at across the session''s interceptions. Sort key for the sessions list, and the upper bound for time-range filters.';
COMMENT ON COLUMN aibridge_sessions.client IS 'The client that issued the session. Scalar rather than an array because a session_id originates from one client.';

-- Serves ORDER BY last_active_at DESC, session_id DESC LIMIT n for the ListAIBridgeSessions query.
CREATE INDEX idx_aibridge_sessions_last_active
    ON aibridge_sessions (last_active_at DESC, session_id DESC);
-- `started_at` is deliberately left unindexed for now, as indexing it doesn't seem to provide much benefit.
-- Revisit later if necessary.

-- Answers the initiator and client filters.
CREATE INDEX idx_aibridge_sessions_initiator
    ON aibridge_sessions (initiator_id);
CREATE INDEX idx_aibridge_sessions_client
    ON aibridge_sessions (client);

-- Answers the array membership filters.
CREATE INDEX idx_aibridge_sessions_providers
    ON aibridge_sessions USING gin (providers);
CREATE INDEX idx_aibridge_sessions_provider_names
    ON aibridge_sessions USING gin (provider_names);
CREATE INDEX idx_aibridge_sessions_models
    ON aibridge_sessions USING gin (models);

-- Adds value to arr only when absent, so repeated interceptions with the same
-- provider or model do not grow the arrays without bound.
CREATE FUNCTION aibridge_session_merge_value(arr text[], value text) RETURNS text[]
    LANGUAGE sql
    IMMUTABLE
    AS $$
    SELECT CASE
        WHEN value IS NULL THEN arr
        WHEN arr @> ARRAY[value] THEN arr
        ELSE arr || value
    END;
$$;

-- Upserts the session row when an interception completes.
--
-- Every accumulator is monotonic: last_active_at only moves forward, started_at
-- only moves back, and the arrays only grow. Interceptions can therefore arrive
-- in any order and the row converges on the same values.
CREATE FUNCTION aibridge_session_track_interception() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    INSERT INTO aibridge_sessions (
        session_id, initiator_id, started_at, last_active_at,
        providers, provider_names, models, client
    )
    VALUES (
        NEW.session_id, NEW.initiator_id, NEW.started_at, NEW.ended_at,
        ARRAY[NEW.provider], ARRAY[NEW.provider_name], ARRAY[NEW.model],
        COALESCE(NEW.client, 'Unknown')
    )
    ON CONFLICT (session_id, initiator_id) DO UPDATE SET
        started_at = LEAST(aibridge_sessions.started_at, EXCLUDED.started_at),
        last_active_at = GREATEST(aibridge_sessions.last_active_at, EXCLUDED.last_active_at),
        providers = aibridge_session_merge_value(aibridge_sessions.providers, NEW.provider),
        provider_names = aibridge_session_merge_value(aibridge_sessions.provider_names, NEW.provider_name),
        models = aibridge_session_merge_value(aibridge_sessions.models, NEW.model);
        -- client is deliberately absent: the first interception to complete
        -- sets it and later ones leave it alone, since a session_id comes from
        -- a single client.
    RETURN NULL;
END;
$$;

-- Creates or updates the session row for each completed interception, keeping
-- aibridge_sessions in sync with aibridge_interceptions.
CREATE TRIGGER aibridge_interceptions_track_session
    AFTER INSERT OR UPDATE ON aibridge_interceptions
    FOR EACH ROW
    WHEN (NEW.ended_at IS NOT NULL)
    EXECUTE FUNCTION aibridge_session_track_interception();

-- Backfills sessions from existing interceptions.
INSERT INTO aibridge_sessions (
    session_id, initiator_id, started_at, last_active_at,
    providers, provider_names, models, client
)
SELECT
    ai.session_id,
    ai.initiator_id,
    MIN(ai.started_at),
    MAX(ai.ended_at),
    ARRAY_AGG(DISTINCT ai.provider ORDER BY ai.provider),
    ARRAY_AGG(DISTINCT ai.provider_name ORDER BY ai.provider_name),
    ARRAY_AGG(DISTINCT ai.model ORDER BY ai.model),
    COALESCE((ARRAY_AGG(ai.client ORDER BY ai.started_at, ai.id))[1], 'Unknown')
FROM aibridge_interceptions ai
WHERE ai.ended_at IS NOT NULL
GROUP BY ai.session_id, ai.initiator_id
ON CONFLICT (session_id, initiator_id) DO NOTHING;
