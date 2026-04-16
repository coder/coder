-- Fixture for migration 000471_chat_acl_sharing.
-- Exercises:
--   * A chat with empty ACLs (default case).
--   * A chat with a user-only ACL.
--   * A chat with a group-only ACL.
--   * A chat with both user and group ACLs.
--   * A root chat + one sub-chat so the chats_with_acl view has data
--     to exercise the COALESCE fallback.
--   * Organizations in each of the three shareable_chat_owners modes.

-- New organizations in 'none' and 'service_accounts' modes. The
-- default organization row (bb640d07-ca8a-4869-b6bc-ae61ebb2fda1,
-- seeded by 000022) keeps the 'everyone' default set by this
-- migration's DEFAULT clause.
INSERT INTO organizations (
    id,
    name,
    display_name,
    description,
    created_at,
    updated_at,
    is_default,
    icon,
    shareable_chat_owners
) VALUES
    (
        'dfc8ee1a-62a1-4f42-bd2a-9a7b3d9a5f0c',
        'chat-acl-sharing-none',
        'Chat Sharing Disabled',
        'Fixture org with shareable_chat_owners = none.',
        '2024-10-01 00:00:00+00',
        '2024-10-01 00:00:00+00',
        false,
        '',
        'none'
    ),
    (
        'c78aed0b-7b0e-4c54-b66a-a85e1ba5b92f',
        'chat-acl-sharing-svc',
        'Chat Sharing Service Accounts Only',
        'Fixture org with shareable_chat_owners = service_accounts.',
        '2024-10-01 00:00:00+00',
        '2024-10-01 00:00:00+00',
        false,
        '',
        'service_accounts'
    )
ON CONFLICT DO NOTHING;

-- Root chat with a user-only ACL.
INSERT INTO chats (
    id,
    owner_id,
    organization_id,
    last_model_config_id,
    title,
    status,
    created_at,
    updated_at,
    user_acl,
    group_acl
)
SELECT
    'aaaaaaaa-0000-0000-0000-000000000001',
    id,
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    '9af5f8d5-6a57-4505-8a69-3d6c787b95fd',
    'ACL Fixture - user only',
    'completed',
    '2024-10-01 00:00:00+00',
    '2024-10-01 00:00:00+00',
    '{"00000000-0000-0000-0000-0000000000aa": {"permissions": ["read"]}}'::jsonb,
    '{}'::jsonb
FROM users
ORDER BY created_at, id
LIMIT 1;

-- Root chat with a group-only ACL.
INSERT INTO chats (
    id,
    owner_id,
    organization_id,
    last_model_config_id,
    title,
    status,
    created_at,
    updated_at,
    user_acl,
    group_acl
)
SELECT
    'aaaaaaaa-0000-0000-0000-000000000002',
    id,
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    '9af5f8d5-6a57-4505-8a69-3d6c787b95fd',
    'ACL Fixture - group only',
    'completed',
    '2024-10-01 00:00:00+00',
    '2024-10-01 00:00:00+00',
    '{}'::jsonb,
    '{"00000000-0000-0000-0000-0000000000bb": {"permissions": ["read"]}}'::jsonb
FROM users
ORDER BY created_at, id
LIMIT 1;

-- Root chat with both user and group ACLs.
INSERT INTO chats (
    id,
    owner_id,
    organization_id,
    last_model_config_id,
    title,
    status,
    created_at,
    updated_at,
    user_acl,
    group_acl
)
SELECT
    'aaaaaaaa-0000-0000-0000-000000000003',
    id,
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    '9af5f8d5-6a57-4505-8a69-3d6c787b95fd',
    'ACL Fixture - user and group',
    'completed',
    '2024-10-01 00:00:00+00',
    '2024-10-01 00:00:00+00',
    '{"00000000-0000-0000-0000-0000000000aa": {"permissions": ["read"]}}'::jsonb,
    '{"00000000-0000-0000-0000-0000000000bb": {"permissions": ["read"]}}'::jsonb
FROM users
ORDER BY created_at, id
LIMIT 1;

-- Root chat with an ACL, plus a sub-chat whose root_chat_id points
-- at the root. The view's COALESCE fallback is exercised by the
-- sub-chat row.
INSERT INTO chats (
    id,
    owner_id,
    organization_id,
    last_model_config_id,
    title,
    status,
    created_at,
    updated_at,
    user_acl,
    group_acl
)
SELECT
    'aaaaaaaa-0000-0000-0000-000000000004',
    id,
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    '9af5f8d5-6a57-4505-8a69-3d6c787b95fd',
    'ACL Fixture - root for sub-chat',
    'completed',
    '2024-10-01 00:00:00+00',
    '2024-10-01 00:00:00+00',
    '{"00000000-0000-0000-0000-0000000000aa": {"permissions": ["read"]}}'::jsonb,
    '{}'::jsonb
FROM users
ORDER BY created_at, id
LIMIT 1;

INSERT INTO chats (
    id,
    owner_id,
    organization_id,
    last_model_config_id,
    title,
    status,
    created_at,
    updated_at,
    parent_chat_id,
    root_chat_id,
    user_acl,
    group_acl
)
SELECT
    'aaaaaaaa-0000-0000-0000-000000000005',
    id,
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    '9af5f8d5-6a57-4505-8a69-3d6c787b95fd',
    'ACL Fixture - sub-chat inheriting ACL',
    'completed',
    '2024-10-01 00:00:00+00',
    '2024-10-01 00:00:00+00',
    'aaaaaaaa-0000-0000-0000-000000000004',
    'aaaaaaaa-0000-0000-0000-000000000004',
    '{}'::jsonb,
    '{}'::jsonb
FROM users
ORDER BY created_at, id
LIMIT 1;
