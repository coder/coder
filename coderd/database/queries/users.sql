-- name: GetUserByID :one
SELECT
	*
FROM
	users
WHERE
	id = $1
LIMIT
	1;

-- name: GetUserByEmailOrUsername :one
SELECT
	*
FROM
	users
WHERE
	LOWER(username) = LOWER(@username)
	OR email = @email
LIMIT
	1;

-- name: GetUserCount :one
SELECT
	COUNT(*)
FROM
	users;

-- name: InsertUser :one
INSERT INTO
	users (
		id,
		email,
		"name",
		login_type,
		revoked,
		hashed_password,
		created_at,
		updated_at,
		username
	)
VALUES
	($1, $2, $3, $4, FALSE, $5, $6, $7, $8) RETURNING *;

-- name: UpdateUserProfile :one
UPDATE
	users
SET
	email = $2,
	"name" = $3,
	username = $4,
	updated_at = $5
WHERE
	id = $1 RETURNING *;

-- name: GetUsers :many
SELECT
	*
FROM
	users;
