-- Create placeholder organization-member system roles for existing
-- organizations. Also add a trigger that creates the placeholder role
-- when an organization is created. Permissions will be empty until
-- populated by the reconciliation routine.
--
-- Note: why do all this in the database (as opposed to coderd)? Less
-- room for race conditions. If the role doesn't exist when coderd
-- expects it, the only correct option is to panic. On the other hand,
-- a placeholder role with empty permissions is harmless and the
-- reconciliation process is idempotent.

-- Create roles for the existing organizations.
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
    NOT EXISTS (
        SELECT 1
        FROM custom_roles
        WHERE
            custom_roles.name = 'organization-member'
            AND custom_roles.organization_id = organizations.id
    );

-- When we insert a new organization, we also want to create a
-- placeholder org-member system role for it.
CREATE OR REPLACE FUNCTION insert_org_member_system_role() RETURNS trigger AS $$
BEGIN
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
    ) VALUES (
        'organization-member',
        '',
        NEW.id,
        '[]'::jsonb,
        '[]'::jsonb,
        '[]'::jsonb,
        '[]'::jsonb,
        true,
        NOW(),
        NOW()
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_insert_org_member_system_role
    AFTER INSERT ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION insert_org_member_system_role();
