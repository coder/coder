-- Seed a legacy global MCP server config immediately before migration 000551.
INSERT INTO organizations (
    id, name, description, created_at, updated_at, is_default, display_name,
    icon, deleted, default_org_member_roles
) VALUES
    (
        '94f7df20-0d04-4dd7-97e9-7c276c337a01', 'mcp-fixture-org', '',
        '2026-01-02 00:00:00+00', '2026-01-02 00:00:00+00', false,
        'MCP Fixture Org', '', false, '{}'
    ),
    (
        '94f7df20-0d04-4dd7-97e9-7c276c337a02', 'deleted-mcp-fixture-org', '',
        '2026-01-02 00:00:00+00', '2026-01-02 00:00:00+00', false,
        'Deleted MCP Fixture Org', '', true, '{}'
    );

INSERT INTO organization_members (
    user_id, organization_id, created_at, updated_at, roles
) VALUES (
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    '94f7df20-0d04-4dd7-97e9-7c276c337a01',
    '2026-01-02 00:00:00+00', '2026-01-02 00:00:00+00', '{}'
);

INSERT INTO dbcrypt_keys (
    number, active_key_digest, created_at, test
) VALUES (
    551, '6a193b3', '2026-01-02 00:00:00+00',
    'JAr/OTfEYiZmArZ/9yZpmeM/KSVCy6eJRZ1/qgKXqonr'
);

INSERT INTO mcp_server_configs (
    id, display_name, slug, description, icon_url, transport, url, auth_type,
    oauth2_client_id, oauth2_client_secret, oauth2_client_secret_key_id,
    oauth2_auth_url, oauth2_token_url, oauth2_scopes, api_key_header,
    api_key_value, api_key_value_key_id, custom_headers, custom_headers_key_id,
    tool_allow_list, tool_deny_list, availability, enabled, created_by,
    updated_by, created_at, updated_at, model_intent, allow_in_plan_mode,
    forward_coder_headers, oauth2_revocation_url
) VALUES (
    '94f7df20-0d04-4dd7-97e9-7c276c337a11', 'MCP Fixture', 'mcp-fixture',
    'fixture description', 'https://example.com/icon.svg', 'streamable_http',
    'https://example.com/mcp', 'oauth2', 'fixture-client',
    'xjUqHoLAQ0WOsBTTI/lfya1NP+hTqOz+X0nhsf0Ru006XAJ19XsAnw==', '6a193b3',
    'https://example.com/authorize', 'https://example.com/token', 'read write',
    'X-API-Key', 'WNKdRi1v1kmlaip3PtpwoDIHsRNaKYQ1jvhN2MRxzUlC2tg=', '6a193b3',
    'rma8zlijD4fvJOqg8US4JZ+2aEo1QCPcLqkn1yi6XYPqrkOVbn7or8Df60mKfVL2zmee5+Ft',
    '6a193b3', ARRAY['allowed'], ARRAY['denied'], 'force_on', true,
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    '2026-01-02 00:00:00+00', '2026-01-03 00:00:00+00', true, true, true,
    'https://example.com/revoke'
);

INSERT INTO mcp_server_user_tokens (
    id, mcp_server_config_id, user_id, access_token, access_token_key_id,
    refresh_token, refresh_token_key_id, token_type, expiry, created_at,
    updated_at, oauth_refresh_failure_reason
) VALUES (
    '94f7df20-0d04-4dd7-97e9-7c276c337a12',
    '94f7df20-0d04-4dd7-97e9-7c276c337a11',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    'WFA88JW4rWYFjoc2qyOJec37Iitq4SlpacETiH+qHwFg+qAEwmqVjg==', '6a193b3',
    'G01YJJHbtDS+Zh2re1a7mfx9v4Ea7rmabzP4rbRdsHiAZ56BI+cg2cI=', '6a193b3',
    'Bearer', '2027-01-01 00:00:00+00', '2026-01-02 00:00:00+00',
    '2026-01-03 00:00:00+00', 'invalid_grant'
);

UPDATE chats
SET mcp_server_ids = ARRAY[
    '94f7df20-0d04-4dd7-97e9-7c276c337a11'::uuid,
    '94f7df20-0d04-4dd7-97e9-7c276c337aff'::uuid,
    '94f7df20-0d04-4dd7-97e9-7c276c337a11'::uuid
]
WHERE id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';

INSERT INTO chat_model_configs (
    id, model, display_name, enabled, is_default, deleted, context_limit,
    compression_threshold, options, ai_provider_id, created_at, updated_at,
    organization_id, user_acl, group_acl
)
SELECT
    '94f7df20-0d04-4dd7-97e9-7c276c337a13', model, display_name, enabled,
    false, deleted, context_limit, compression_threshold, options,
    ai_provider_id, created_at, updated_at,
    '94f7df20-0d04-4dd7-97e9-7c276c337a02', '{}'::jsonb, '{}'::jsonb
FROM chat_model_configs
WHERE id = (
    SELECT last_model_config_id
    FROM chats
    WHERE id = '72c0438a-18eb-4688-ab80-e4c6a126ef96'
);

INSERT INTO chats (
    id, owner_id, last_model_config_id, organization_id, mcp_server_ids
)
SELECT
    '72c0438a-18eb-4688-ab80-e4c6a126ef98', owner_id,
    '94f7df20-0d04-4dd7-97e9-7c276c337a13',
    '94f7df20-0d04-4dd7-97e9-7c276c337a02',
    ARRAY['94f7df20-0d04-4dd7-97e9-7c276c337a11'::uuid]
FROM chats
WHERE id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';
