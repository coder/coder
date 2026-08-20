-- Keeps an MCP config, token, and chat to exercise organization_id and its
-- foreign keys through later migrations and the down sweep. Fixtures run
-- after their matching migration, so this row already has organization_id.

INSERT INTO organizations (
    id,
    name,
    display_name,
    description,
    icon,
    created_at,
    updated_at,
    is_default,
    deleted,
    default_org_member_roles
) VALUES (
    'f5610000-0000-4000-8000-000000000001',
    'fixture-mcp-org',
    'Fixture MCP Org',
    '',
    '',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00',
    FALSE,
    FALSE,
    '{}'
);

INSERT INTO ai_providers (
    id,
    type,
    name,
    display_name,
    enabled,
    base_url,
    created_at,
    updated_at
) VALUES (
    'f5610000-0000-4000-8000-000000000003',
    'openai',
    'fixture-mcp-ai-provider',
    'Fixture MCP AI Provider',
    TRUE,
    'https://example.com',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);

INSERT INTO chat_model_configs (
    id,
    model,
    display_name,
    ai_provider_id,
    context_limit,
    compression_threshold,
    created_at,
    updated_at
) VALUES (
    'f5610000-0000-4000-8000-000000000004',
    'fixture-model',
    'Fixture Model',
    'f5610000-0000-4000-8000-000000000003',
    128000,
    70,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);

INSERT INTO dbcrypt_keys (number, active_key_digest, test)
VALUES (561000, 'fixture-000561-key-digest', 'fixture-000561');

-- MCP server config with a ciphertext-shaped secret pair (value + key ID)
-- to assert the backfill leaves secret columns byte-identical. The key ID
-- references dbcrypt_keys(active_key_digest).
INSERT INTO mcp_server_configs (
    id,
    organization_id,
    display_name,
    slug,
    url,
    auth_type,
    api_key_value,
    api_key_value_key_id,
    availability,
    enabled,
    created_by,
    updated_by,
    created_at,
    updated_at
)
SELECT
    'f5610000-0000-4000-8000-000000000005',
    (SELECT id FROM organizations WHERE is_default = true LIMIT 1),
    'Fixture Org Backfill MCP Server',
    'fixture-org-backfill-mcp-server',
    'https://mcp.example.com/org-backfill',
    'api_key',
    'fixture-ciphertext',
    'fixture-000561-key-digest',
    'default_on',
    TRUE,
    u.id,
    u.id,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
FROM users u
ORDER BY u.created_at, u.id
LIMIT 1;

INSERT INTO mcp_server_user_tokens (
    id,
    mcp_server_config_id,
    user_id,
    access_token,
    token_type,
    created_at,
    updated_at
)
SELECT
    'f5610000-0000-4000-8000-000000000006',
    'f5610000-0000-4000-8000-000000000005',
    id,
    'fixture-org-backfill-access-token',
    'Bearer',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
FROM users
ORDER BY created_at, id
LIMIT 1;

INSERT INTO chats (
    id,
    owner_id,
    organization_id,
    last_model_config_id,
    title,
    mcp_server_ids,
    created_at,
    updated_at
) VALUES (
    'f5610000-0000-4000-8000-000000000007',
    (SELECT id FROM users ORDER BY created_at, id LIMIT 1),
    'f5610000-0000-4000-8000-000000000001',
    'f5610000-0000-4000-8000-000000000004',
    'Fixture MCP Org Backfill Chat',
    '{f5610000-0000-4000-8000-000000000005}'::uuid[],
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);
