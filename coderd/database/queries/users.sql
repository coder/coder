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


-- name: PaginatedUsersAfter :many
SELECT
	*
FROM
	users
WHERE
	CASE
		WHEN @after::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			created_at > (SELECT created_at FROM users WHERE id = @after)
		-- If the after field is not provided, just return the first page
		ELSE true
	END
	AND
	CASE
	    WHEN @email::text != '' THEN
				email LIKE '%' || @email || '%'
		ELSE true
	END
ORDER BY
	created_at ASC
LIMIT
	@limit_opt;

-- name: PaginatedUsersBefore :many
SELECT users_before.* FROM
	(SELECT
		*
	FROM
		users
	WHERE
		CASE
			WHEN @before::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
				created_at < (SELECT created_at FROM users WHERE id = @before)
			-- If the 'before' field is not provided, this will return the last page.
			-- Kinda odd, it's just a consequence of spliting the pagination queries into 2
			-- functions.
			ELSE true
		END
		AND
		CASE
			WHEN @email::text != '' THEN
				email LIKE '%' || @email || '%'
			ELSE true
		END
	ORDER BY
		created_at DESC
	LIMIT
		@limit_opt) AS users_before
-- Maintain the original ordering of the rows so the pages are the same order
-- as PaginatedUsersAfter.
ORDER BY users_before.created_at ASC;
