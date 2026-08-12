-- Inserts a completed interception so the trigger added in this migration
-- populates aibridge_sessions.
--
-- The interception carries an ended_at because only completed interceptions are
-- tracked.
INSERT INTO
    aibridge_interceptions (
        id,
        initiator_id,
        provider,
        provider_name,
        model,
        client,
        client_session_id,
        started_at,
        ended_at
    )
VALUES (
        '1f7c4a5e-6b2d-4c8f-9a1e-2d3b4c5e6f70',
        '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com, from 000022_initial_v0.6.6.up.sql
        'anthropic',
        'anthropic-prod',
        'claude-sonnet-4-6',
        'claude-code',
        'fixture-session-1',
        '2025-09-15 12:45:13.921148+00',
        '2025-09-15 12:45:21.674413+00'
    );
