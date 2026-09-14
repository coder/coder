ALTER TABLE custom_roles
	-- This column is nullable, meaning no organization scope
	ADD COLUMN organization_id uuid;

COMMENT ON COLUMN custom_roles.organization_id IS 'Roles can optionally be scoped to an organization'
