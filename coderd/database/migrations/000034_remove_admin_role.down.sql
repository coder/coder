UPDATE
	users
SET
	-- Replace 'template-admin' and 'user-admin' role with 'admin'
	rbac_roles = array_append(
		array_remove(
			array_remove(users.rbac_roles, 'template-admin'),
			'user-admin'
		), 'admin')
WHERE
	-- Only on existing admins
	ARRAY ['template-admin', 'user-admin'] <@ rbac_roles;


UPDATE
	users
SET
	-- Replace 'owner' with 'admin
	rbac_roles = array_replace(rbac_roles, 'owner', 'admin')
WHERE
	-- Only on the owner
	'owner' = ANY(rbac_roles);
