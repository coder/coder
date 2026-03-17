DELETE FROM custom_roles
    WHERE name = 'organization-service-account' AND is_system = true;

ALTER TABLE organizations
    ADD COLUMN workspace_sharing_disabled boolean NOT NULL DEFAULT false;

-- Migrate back: 'none' -> disabled, everything else -> enabled.
UPDATE organizations
    SET workspace_sharing_disabled = true
    WHERE shareable_workspace_owners = 'none';

ALTER TABLE organizations DROP COLUMN shareable_workspace_owners;

DROP TYPE shareable_workspace_owners;

-- Restore the original single-role trigger from migration 408.
DROP TRIGGER IF EXISTS trigger_insert_organization_system_roles ON organizations;
DROP FUNCTION IF EXISTS insert_organization_system_roles;

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
