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
		WHEN @after_user :: uuid != '00000000-00000000-00000000-00000000' THEN (
		    	-- The pagination cursor is the last user of the previous page.
		    	-- The query is ordered by the created_at field, so select all
		    	-- users after the cursor. We also want to include any users
		    	-- that share the created_at (super rare).
				created_at >= (
					SELECT
						created_at
					FROM
						users
					WHERE
						id = @after_user
				)
				-- Omit the cursor from the final.
				AND id != @after_user
			)
			ELSE true
	END
	AND CASE
		WHEN @search :: text != '' THEN (
			email LIKE concat('%', @search, '%')
			OR username LIKE concat('%', @search, '%')
			OR 'name' LIKE concat('%', @search, '%')
		)
		ELSE true
	END
ORDER BY
    -- Deterministic and consistent ordering of all users, even if they share
    -- a timestamp. This is to ensure consistent pagination.
	(created_at, id) ASC OFFSET @offset_opt
LIMIT
	-- A null limit means "no limit", so -1 means return all
	NULLIF(@limit_opt :: int, -1);
