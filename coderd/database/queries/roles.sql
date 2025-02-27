-- name: CustomRoles :many
SELECT
	*
FROM
	custom_roles
WHERE
	true
	-- @lookup_roles will filter for exact (role_name, org_id) pairs
	-- To do this manually in SQL, you can construct an array and cast it:
	-- cast(ARRAY[('customrole','ece79dac-926e-44ca-9790-2ff7c5eb6e0c')] AS name_organization_pair[])
	AND CASE WHEN array_length(@lookup_roles :: name_organization_pair[], 1) > 0  THEN
		-- Using 'coalesce' to avoid troubles with null literals being an empty string.
		(name, coalesce(organization_id, '00000000-0000-0000-0000-000000000000' ::uuid)) = ANY (@lookup_roles::name_organization_pair[])
	ELSE true
	END
	-- This allows fetching all roles, or just site wide roles
	AND CASE WHEN @exclude_org_roles :: boolean  THEN
		organization_id IS null
	ELSE true
	END
	-- Allows fetching all roles to a particular organization
	AND CASE WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid  THEN
		organization_id = @organization_id
	ELSE true
	END
;

-- name: DeleteCustomRole :exec
DELETE FROM
	custom_roles
WHERE
	name = lower(@name)
	AND organization_id = @organization_id
;

-- name: InsertCustomRole :one
INSERT INTO
	custom_roles (
	name,
	display_name,
	organization_id,
	site_permissions,
	org_permissions,
	user_permissions,
	created_at,
	updated_at
)
VALUES (
	-- Always force lowercase names
	lower(@name),
	@display_name,
	@organization_id,
	@site_permissions,
	@org_permissions,
	@user_permissions,
	now(),
	now()
)
RETURNING *;

-- name: UpdateCustomRole :one
UPDATE
	custom_roles
SET
	display_name = @display_name,
	site_permissions = @site_permissions,
	org_permissions = @org_permissions,
	user_permissions = @user_permissions,
	updated_at = now()
WHERE
	name = lower(@name)
	AND organization_id = @organization_id
RETURNING *;
