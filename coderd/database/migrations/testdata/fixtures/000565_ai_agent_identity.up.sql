INSERT INTO users (
    id,
    email,
    username,
    hashed_password,
    created_at,
    updated_at,
    status,
    rbac_roles,
    login_type,
    kind
)
VALUES
(
    '56500000-0000-0000-0000-000000000001',
    'fixture-ai-owner@example.com',
    'fixture-ai-owner',
    ''::bytea,
    '2026-01-01 00:00:00+00',
    '2026-01-01 00:00:00+00',
    'active',
    '{}',
    'password',
    'human'
),
(
    '56500000-0000-0000-0000-000000000002',
    '',
    'ai-chat-56500000',
    ''::bytea,
    '2026-01-01 00:00:00+00',
    '2026-01-01 00:00:00+00',
    'active',
    '{}',
    'none',
    'ai_agent'
);

INSERT INTO ai_agents (
    user_id,
    owner_user_id,
    origin_type,
    origin_id,
    created_at,
    deleted
)
VALUES (
    '56500000-0000-0000-0000-000000000002',
    '56500000-0000-0000-0000-000000000001',
    'chat',
    '56500000-0000-0000-0000-000000000003',
    '2026-01-01 00:00:00+00',
    false
);
