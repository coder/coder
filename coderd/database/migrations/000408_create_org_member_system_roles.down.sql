-- Drop the trigger and function created by the up migration.
DROP TRIGGER IF EXISTS trigger_insert_org_member_system_role ON organizations;
DROP FUNCTION IF EXISTS insert_org_member_system_role;

-- Remove organization-member system roles created by the up migration.
DELETE FROM custom_roles WHERE name = 'organization-member' AND is_system = true;
