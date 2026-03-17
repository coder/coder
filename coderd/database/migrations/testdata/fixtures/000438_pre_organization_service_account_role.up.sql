-- Fixture for migration 000443_three_options_for_allowed_workspace_sharing.
-- Inserts a custom role named 'Organization-Service-Account' (mixed case)
-- to ensure the migration's case-insensitive rename catches it.
INSERT INTO custom_roles (
    name,
    display_name,
    organization_id,
    site_permissions,
    org_permissions,
    user_permissions,
    member_permissions,
    is_system,
    created_at,
    updated_at
)
VALUES (
    'Organization-Service-Account',
    'User-created role',
    'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
    '[]'::jsonb,
    '[]'::jsonb,
    '[]'::jsonb,
    '[]'::jsonb,
    false,
    NOW(),
    NOW()
)
ON CONFLICT DO NOTHING;
