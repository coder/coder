INSERT INTO chat_mcp_servers (
    id,
    chat_id,
    slug,
    url,
    headers,
    tool_allow_list,
    tool_deny_list,
    allow_in_subagents,
    forward_coder_headers
)
SELECT
    '2f7c1e7a-6d3b-4b7e-9a10-5c0e8f3d2b41',
    c.id,
    'fixture-bot',
    'https://mcp.example.com/mcp',
    '{}',
    '{}',
    '{}',
    false,
    false
FROM (
    SELECT id FROM chats ORDER BY created_at, id LIMIT 1
) AS c;
