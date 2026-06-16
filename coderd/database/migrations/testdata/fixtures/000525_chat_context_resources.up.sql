-- Pinned context resources covering each non-reserved body kind plus a
-- non-OK status. The earlier chat fixtures already insert at least one row
-- into chats; we attach the resources to the first such chat (ordered
-- deterministically) so migration tests see a non-empty
-- chat_context_resources table without hard-coding a specific chat ID.
INSERT INTO chat_context_resources (
    chat_id,
    source,
    body_kind,
    body,
    content_hash,
    size_bytes,
    status,
    error,
    source_path
)
SELECT
    c.id,
    v.source,
    v.body_kind::workspace_agent_context_body_kind,
    v.body::jsonb,
    decode(v.content_hash, 'hex'),
    v.size_bytes,
    v.status::workspace_agent_context_resource_status,
    v.error,
    v.source_path
FROM (
    SELECT id FROM chats ORDER BY created_at, id LIMIT 1
) AS c
CROSS JOIN (
    VALUES
        (
            '/home/coder/workspace/AGENTS.md',
            'instruction_file',
            '{"content":"aGVsbG8="}',
            '1111111111111111111111111111111111111111111111111111111111111111',
            5::bigint,
            'ok',
            '',
            ''
        ),
        (
            '/home/coder/workspace/.agents/skills/example/SKILL.md',
            'skill',
            '{"meta":"LS0tCm5hbWU6IGV4YW1wbGUKLS0tCmJvZHk=","name":"example","description":"Example skill"}',
            '2222222222222222222222222222222222222222222222222222222222222222',
            32::bigint,
            'ok',
            '',
            '/home/coder/workspace'
        ),
        (
            '/home/coder/workspace/.mcp.json',
            'mcp_config',
            '{}',
            '3333333333333333333333333333333333333333333333333333333333333333',
            128::bigint,
            'ok',
            '',
            ''
        ),
        (
            'mcp:echo',
            'mcp_server',
            '{"server_name":"echo","description":"echoes input"}',
            '4444444444444444444444444444444444444444444444444444444444444444',
            256::bigint,
            'ok',
            '',
            '/home/coder/workspace/.mcp.json'
        ),
        (
            '/home/coder/workspace/big.md',
            'instruction_file',
            '{}',
            '5555555555555555555555555555555555555555555555555555555555555555',
            99999::bigint,
            'oversize',
            'file exceeds 64KiB per-resource cap',
            ''
        )
) AS v(source, body_kind, body, content_hash, size_bytes, status, error, source_path);
