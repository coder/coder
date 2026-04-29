-- No FK to boundary_sessions: Bridge interceptions may be recorded before
-- the boundary_sessions row exists, since boundary log delivery is async.
-- boundary_session_id is a soft reference resolved at query time.
ALTER TABLE aibridge_interceptions
    ADD COLUMN boundary_session_id UUID NULL,
    ADD COLUMN boundary_sequence_number BIGINT NULL;

COMMENT ON COLUMN aibridge_interceptions.boundary_session_id IS
    'The Boundary session ID, linking this Bridge interception to a Boundary confinement session.';
COMMENT ON COLUMN aibridge_interceptions.boundary_sequence_number IS
    'The Boundary sequence number from the request header. Used to determine exact ordering of network requests relative to Boundary audit events. NULL when the request did not pass through Boundary.';

CREATE INDEX idx_aibridge_interceptions_boundary_session_id
    ON aibridge_interceptions (boundary_session_id)
    WHERE boundary_session_id IS NOT NULL;
