-- Serves the AI Bridge sessions list. The list needs one row per session,
-- ordered by the session's latest interception, which an anti-join finds by
-- probing for a newer interception in the same session. Ordering the index by
-- (session_id, initiator_id, started_at DESC, id DESC) makes that probe an
-- index-only lookup, so the ordered scan of the list can stop after LIMIT
-- sessions instead of aggregating every interception.
--
-- Partial on ended_at because the list excludes in-flight interceptions.
CREATE INDEX idx_aibridge_interceptions_session_latest
    ON aibridge_interceptions (session_id, initiator_id, started_at DESC, id DESC)
    WHERE ended_at IS NOT NULL;
