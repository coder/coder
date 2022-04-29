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
		username,
		hashed_password,
		created_at,
		updated_at,
		rbac_roles
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: UpdateUserProfile :one
UPDATE
	users
SET
	email = $2,
	username = $3,
	updated_at = $4
WHERE
	id = $1 RETURNING *;

-- name: UpdateUserRoles :one
UPDATE
    users
SET
	-- Remove all duplicates from the roles.
	rbac_roles = ARRAY(SELECT DISTINCT UNNEST(@granted_roles :: text[]))
WHERE
 	id = @id
RETURNING *;

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
	-- Start filters
	-- Filter by name, email or username
	AND CASE
		WHEN @search :: text != '' THEN (
			email LIKE concat('%', @search, '%')
			OR username LIKE concat('%', @search, '%')
		)	
		ELSE true
	END
	-- Filter by status
	AND CASE
		-- @status needs to be a text because it can be empty, If it was
		-- user_status enum, it would not.
		WHEN @status :: text != '' THEN (
			status = @status :: user_status
		)
		ELSE true
	END
	-- End of filters
ORDER BY
    -- Deterministic and consistent ordering of all users, even if they share
    -- a timestamp. This is to ensure consistent pagination.
	(created_at, id) ASC OFFSET @offset_opt
LIMIT
	-- A null limit means "no limit", so -1 means return all
	NULLIF(@limit_opt :: int, -1);

-- name: UpdateUserStatus :one
UPDATE
	users
SET
	status = $2,
	updated_at = $3
WHERE
	id = $1 RETURNING *;
