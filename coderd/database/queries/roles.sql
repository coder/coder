-- name: CustomRoles :many
SELECT
	*
FROM
	custom_roles
WHERE
  true
  -- Lookup roles filter expects the role names to be in the rbac package
  -- format. Eg: name[:<organization_id>]
  AND CASE WHEN array_length(@lookup_roles :: text[], 1) > 0  THEN
	-- Case insensitive lookup with org_id appended (if non-null).
    -- This will return just the name if org_id is null. It'll append
    -- the org_id if not null
	concat(name, NULLIF(concat(':', organization_id), ':')) ILIKE ANY(@lookup_roles :: text [])
    ELSE true
  END
  -- Org scoping filter, to only fetch site wide roles
  AND CASE WHEN @exclude_org_roles :: boolean  THEN
	organization_id IS null
	ELSE true
  END
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
