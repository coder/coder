-- Fixture for migration 000433_add_is_service_account_to_users.
-- Inserts multiple service accounts with empty emails to help test
-- the down migration, which must assign each a unique placeholder
-- email before restoring the original unique index on email.

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
    is_service_account
)
VALUES (
    'b2ce097d-2287-4d64-a550-ed821969545d',
    '',
    'fixture-service-account-1',
    '',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00',
    'active',
    '{}',
    'none',
    true
),
(
    '3e218a4a-3b4a-4242-b24e-9430277e619d',
    '',
    'fixture-service-account-2',
    '',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00',
    'active',
    '{}',
    'none',
    true
);
