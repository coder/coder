-- name: CustomRoles :many
SELECT
	*
FROM
	custom_roles
WHERE
	-- Case insensitive
	name ILIKE ANY(@lookup_roles :: text [])
;
