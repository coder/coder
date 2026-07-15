-- MCP server proposal for the fixture chat from 000424, referencing
-- the personal MCP server config from 000543 as if it had been
-- accepted.
INSERT INTO mcp_server_proposals (
    id,
    chat_id,
    requester_id,
    channel,
    thread_ts,
    message_ts,
    request,
    status,
    mcp_server_config_id,
    created_at,
    accepted_at
) VALUES (
    'd4e5f6a7-b8c9-0123-def1-234567890123',
    '5a4ac6a3-9dc5-440f-ae6b-5805e477bc59',
    '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com
    'C0FIXTURE',
    '1700000000.000100',
    '1700000001.000200',
    '{"display_name": "Fixture Proposal", "slug": "fixture-proposal", "url": "https://mcp.example.com/proposal", "transport": "streamable_http", "auth_type": "none"}',
    'accepted',
    'c3d4e5f6-a7b8-9012-cdef-123456789012',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:05:00+00'
);
