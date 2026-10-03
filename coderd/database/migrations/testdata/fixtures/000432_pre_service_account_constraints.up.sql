-- Fixture for migration 000433_add_is_service_account_to_users.
-- Inserts a user with an empty email to ensure the migration
-- correctly marks them as a service account before adding the
-- users_email_not_empty constraint.

INSERT INTO users (
    id,
    email,
    username,
    hashed_password,
    created_at,
    updated_at,
    status,
    rbac_roles,
    login_type
)
VALUES (
    '8ddb584a-68b8-48ac-998f-86f091ccb380',
    '',
    'fixture-empty-email-user-to-service-account',
    '',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00',
    'active',
    '{}',
    'password'
);
