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
	users
WHERE
	CASE
		-- This allows using the last element on a page as effectively a cursor.
		-- This is an important option for scripts that need to paginate without
		-- duplicating or missing data.
		WHEN @created_after :: timestamp with time zone != '0001-01-01 00:00:00+00' THEN created_at > @created_after
		ELSE true
	END
	AND CASE
		WHEN @search_name :: text != '' THEN (
			email LIKE concat('%', @search_name, '%')
			OR username LIKE concat('%', @search_name, '%')
		)
		ELSE true
	END
ORDER BY
	created_at ASC OFFSET @offset_opt
LIMIT
	-- A null limit means "no limit", so -1 means return all
	NULLIF(@limit_opt :: int, -1);
