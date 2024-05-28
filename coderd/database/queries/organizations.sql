-- name: GetDefaultOrganization :one
SELECT
	*
FROM
	organizations
WHERE
	is_default = true
LIMIT
	1;

-- name: GetOrganizations :many
SELECT
	*
FROM
	organizations;

-- name: GetOrganizationByID :one
SELECT
	*
FROM
	organizations
WHERE
	id = $1;

-- name: GetOrganizationByName :one
SELECT
	*
FROM
	organizations
WHERE
	LOWER("name") = LOWER(@name)
LIMIT
	1;

-- name: GetOrganizationsByUserID :many
SELECT
	*
FROM
	organizations
WHERE
	id = ANY(
		SELECT
			organization_id
		FROM
			organization_members
		WHERE
			user_id = $1
	);

-- name: InsertOrganization :one
INSERT INTO
	organizations (id, "name", description, created_at, updated_at, is_default)
VALUES
	-- If no organizations exist, and this is the first, make it the default.
	($1, $2, $3, $4, $5, (SELECT TRUE FROM organizations LIMIT 1) IS NULL) RETURNING *;

-- name: UpdateOrganization :one
UPDATE
	organizations
SET
	updated_at = @updated_at,
	name = @name
WHERE
	id = @id
RETURNING *;

-- name: DeleteOrganization :exec
DELETE FROM
	organizations
WHERE
	id = $1 AND
	is_default = false;
