-- name: CustomRoles :many
SELECT
	*
FROM
	custom_roles
WHERE
  true
  -- Lookup roles filter
  AND CASE WHEN array_length(@lookup_roles :: text[], 1) > 0  THEN
	-- Case insensitive
	name ILIKE ANY(@lookup_roles :: text [])
    ELSE true
  END
  -- Org scoping filter, to only fetch site wide roles
  AND CASE WHEN @exclude_org_roles :: boolean  THEN
	organization_id IS null
	ELSE true
  END
;

-- name: UpsertCustomRole :one
INSERT INTO
	custom_roles (
	    name,
	    display_name,
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
