CREATE TYPE shareable_workspace_owners AS ENUM ('none', 'everyone', 'service_accounts');

ALTER TABLE organizations
    ADD COLUMN shareable_workspace_owners shareable_workspace_owners NOT NULL DEFAULT 'everyone';

COMMENT ON COLUMN organizations.shareable_workspace_owners IS 'Controls whose workspaces can be shared: none, everyone, or service_accounts.';

-- Migrate existing data from the boolean column.
UPDATE organizations
    SET shareable_workspace_owners = 'none'
    WHERE workspace_sharing_disabled = true;

ALTER TABLE organizations DROP COLUMN workspace_sharing_disabled;

-- Defensively rename any existing 'organization-service-account' roles
-- so they don't collide with the new system role.
UPDATE custom_roles
    SET name = name || '-' || id::text
    -- lower(name) is part of the existing unique index
    WHERE lower(name) = 'organization-service-account';

-- Create skeleton organization-service-account system roles for all
-- existing organizations, mirroring what migration 408 did for
-- organization-member.
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
    'organization-service-account',
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
    organizations;

-- Replace the single-role trigger with one that creates both system
-- roles when a new organization is inserted.
DROP TRIGGER IF EXISTS trigger_insert_org_member_system_role ON organizations;
DROP FUNCTION IF EXISTS insert_org_member_system_role;

CREATE OR REPLACE FUNCTION insert_organization_system_roles() RETURNS trigger AS $$
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
    ) VALUES
    (
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
    ),
    (
        'organization-service-account',
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

CREATE TRIGGER trigger_insert_organization_system_roles
    AFTER INSERT ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION insert_organization_system_roles();
