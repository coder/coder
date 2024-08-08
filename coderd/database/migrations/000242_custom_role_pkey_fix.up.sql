ALTER TABLE custom_roles
	DROP CONSTRAINT custom_roles_pkey;

-- Roles are unique to the organization.
ALTER TABLE custom_roles
	ADD CONSTRAINT custom_roles_unique_key UNIQUE (name, organization_id);
