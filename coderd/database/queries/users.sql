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


-- name: PaginatedUsers :many
SELECT
	*
FROM
	users
WHERE
	CASE
		WHEN @after::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			created_at > (SELECT created_at FROM users WHERE id = @after)
		WHEN @before::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			created_at < (SELECT created_at FROM users WHERE id = @before)
	    ELSE true
	END
ORDER BY
    -- TODO: When doing 'before', we need to flip this to DESC.
	-- You cannot put 'ASC' or 'DESC' in a CASE statement. :'(
	-- Until we figure this out, before is broken.
	-- Another option is to do a subquery above
	created_at ASC
LIMIT
	@limit_opt;
