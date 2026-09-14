ALTER TABLE custom_roles
	-- This column is nullable, meaning no organization scope
	DROP COLUMN organization_id;
