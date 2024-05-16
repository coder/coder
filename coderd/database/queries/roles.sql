-- name: CustomRolesByName :many
SELECT
	*
FROM
	custom_roles
WHERE
	-- Case insensitive
	name ILIKE ANY(@lookup_roles :: text [])
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
