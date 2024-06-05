-- name: CustomRoles :many
SELECT
	*
FROM
	custom_roles
WHERE
  true
  -- @lookup_roles will filter for exact (role_name, org_id) pairs
  AND CASE WHEN array_length(@lookup_roles :: name_organization_pair_list[], 1) > 0  THEN
	(name, organization_id) ILIKE ANY (@lookup_roles::name_organization_pair_list[])
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

-- name: UpsertCustomRole :one
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
ON CONFLICT (name)
	DO UPDATE SET
	display_name = @display_name,
	site_permissions = @site_permissions,
	org_permissions = @org_permissions,
	user_permissions = @user_permissions,
	updated_at = now()
RETURNING *
;
