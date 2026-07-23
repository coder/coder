INSERT INTO boundary_sessions (
    id,
    workspace_agent_id,
    confined_process_name,
    started_at,
    updated_at
) VALUES (
    'a1b2c3d4-e5f6-4890-abcd-ef1234567890',
    '45e89705-e09d-4850-bcec-f9a937f5d78d',
    'claude-code',
    '2026-04-01 10:00:00+00',
    '2026-04-01 10:00:00+00'
);

INSERT INTO boundary_logs (
    id,
    session_id,
    sequence_number,
    captured_at,
    created_at,
    proto,
    method,
    detail,
    matched_rule
) VALUES (
    'b2c3d4e5-f6a7-4901-bcde-f12345678901',
    'a1b2c3d4-e5f6-4890-abcd-ef1234567890',
    0,
    '2026-04-01 10:00:01+00',
    '2026-04-01 10:00:00+00',
    'http',
    'GET',
    'https://api.anthropic.com/v1/messages',
    'domain=api.anthropic.com'
);
