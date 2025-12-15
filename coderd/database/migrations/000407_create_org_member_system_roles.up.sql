-- Create placeholder organization-member system roles for existing
-- organizations.  Permissions are empty here and will be populated by
-- the startup hook.
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
SELECT
    'organization-member', -- reserved role name, so it doesn't exist in DB yet
    '',
    id,
    '[]'::jsonb,
    '[]'::jsonb,
    '[]'::jsonb,
    '[]'::jsonb,
    true,
    NOW(),
    NOW()
FROM
    organizations
WHERE
    deleted = false
    AND NOT EXISTS (
        SELECT 1
        FROM custom_roles
        WHERE
            custom_roles.name = 'organization-member'
            AND custom_roles.organization_id = organizations.id
    );
