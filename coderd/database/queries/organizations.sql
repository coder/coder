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
	id = (
		SELECT
			organization_id
		FROM
			organization_members
		WHERE
			user_id = $1
	);

-- name: InsertOrganization :one
INSERT INTO
	organizations (id, "name", description, created_at, updated_at)
VALUES
	($1, $2, $3, $4, $5) RETURNING *;
