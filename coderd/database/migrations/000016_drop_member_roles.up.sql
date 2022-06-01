--- Remove the now implied 'member' role.
UPDATE
	users
SET
	rbac_roles = array_remove(rbac_roles, 'member');

--- Remove the now implied 'organization-member' role.
UPDATE
	organization_members
SET
	roles = array_remove(roles, 'organization-member:'||organization_id::text);
