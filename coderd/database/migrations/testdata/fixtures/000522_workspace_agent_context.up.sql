-- Snapshot row and a representative set of resources covering each
-- v1 body kind plus a non-OK status. workspace_agent_id matches an
-- existing fixture row from 000507_boundary_sessions_and_logs.
INSERT INTO workspace_agent_context_snapshots (
    workspace_agent_id,
    version,
    aggregate_hash,
    snapshot_error,
    received_at
) VALUES (
    '45e89705-e09d-4850-bcec-f9a937f5d78d',
    1,
    '\x000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f',
    '',
    '2026-06-01 12:00:00+00'
);

INSERT INTO workspace_agent_context_resources (
    workspace_agent_id,
    source,
    body_kind,
    body,
    content_hash,
    size_bytes,
    status,
    error,
    source_path,
    created_at,
    updated_at
) VALUES
(
    '45e89705-e09d-4850-bcec-f9a937f5d78d',
    '/home/coder/workspace/AGENTS.md',
    'instruction_file',
    '{"content":"aGVsbG8="}',
    '\x1111111111111111111111111111111111111111111111111111111111111111',
    5,
    'ok',
    '',
    '',
    '2026-06-01 12:00:00+00',
    '2026-06-01 12:00:00+00'
),
(
    '45e89705-e09d-4850-bcec-f9a937f5d78d',
    '/home/coder/workspace/.agents/skills/example/SKILL.md',
    'skill',
    '{"meta":"LS0tCm5hbWU6IGV4YW1wbGUKLS0tCmJvZHk=","name":"example","description":"Example skill"}',
    '\x2222222222222222222222222222222222222222222222222222222222222222',
    32,
    'ok',
    '',
    '/home/coder/workspace',
    '2026-06-01 12:00:00+00',
    '2026-06-01 12:00:00+00'
),
(
    '45e89705-e09d-4850-bcec-f9a937f5d78d',
    '/home/coder/workspace/.mcp.json',
    'mcp_config',
    '{}',
    '\x3333333333333333333333333333333333333333333333333333333333333333',
    128,
    'ok',
    '',
    '',
    '2026-06-01 12:00:00+00',
    '2026-06-01 12:00:00+00'
),
(
    '45e89705-e09d-4850-bcec-f9a937f5d78d',
    'mcp:echo',
    'mcp_server',
    '{"server_name":"echo","description":"echoes input"}',
    '\x4444444444444444444444444444444444444444444444444444444444444444',
    256,
    'ok',
    '',
    '/home/coder/workspace/.mcp.json',
    '2026-06-01 12:00:00+00',
    '2026-06-01 12:00:00+00'
),
(
    '45e89705-e09d-4850-bcec-f9a937f5d78d',
    '/home/coder/workspace/big.md',
    'instruction_file',
    '{}',
    '\x5555555555555555555555555555555555555555555555555555555555555555',
    99999,
    'oversize',
    'file exceeds 64KiB per-resource cap',
    '',
    '2026-06-01 12:00:00+00',
    '2026-06-01 12:00:00+00'
);
