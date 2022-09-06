-- name: GetUserByID :one
SELECT
	*
FROM
	users
WHERE
	id = $1
LIMIT
	1;

-- name: GetUsersByIDs :many
SELECT * FROM users WHERE id = ANY(@ids :: uuid [ ]);

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

-- name: GetActiveUserCount :one
SELECT
	COUNT(*)
FROM
	users
WHERE
    status = 'active'::public.user_status;

-- name: InsertUser :one
INSERT INTO
	users (
		id,
		email,
		username,
		hashed_password,
		created_at,
		updated_at,
		rbac_roles,
		login_type
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: UpdateUserProfile :one
UPDATE
	users
SET
	email = $2,
	username = $3,
	avatar_url = $4,
	updated_at = $5
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

-- name: UpdateUserHashedPassword :exec
UPDATE
	users
SET
	hashed_password = $2
WHERE
	id = $1;

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
		WHEN @after_id :: uuid != '00000000-00000000-00000000-00000000' THEN (
			-- The pagination cursor is the last ID of the previous page.
			-- The query is ordered by the created_at field, so select all
			-- rows after the cursor.
			(created_at, id) > (
				SELECT
					created_at, id
				FROM
					users
				WHERE
					id = @after_id
			)
		)
		ELSE true
	END
	-- Start filters
	-- Filter by name, email or username
	AND CASE
		WHEN @search :: text != '' THEN (
			email ILIKE concat('%', @search, '%')
			OR username ILIKE concat('%', @search, '%')
		)
		ELSE true
	END
	-- Filter by status
	AND CASE
		-- @status needs to be a text because it can be empty, If it was
		-- user_status enum, it would not.
		WHEN cardinality(@status :: user_status[]) > 0 THEN
			status = ANY(@status :: user_status[])
		ELSE true
	END
	-- Filter by rbac_roles
	AND CASE
		-- @rbac_role allows filtering by rbac roles. If 'member' is included, show everyone, as
	    -- everyone is a member.
		WHEN cardinality(@rbac_role :: text[]) > 0 AND 'member' != ANY(@rbac_role :: text[]) THEN
		    rbac_roles && @rbac_role :: text[]
		ELSE true
	END
	-- End of filters
ORDER BY
	-- Deterministic and consistent ordering of all users, even if they share
	-- a timestamp. This is to ensure consistent pagination.
	(created_at, id) ASC OFFSET @offset_opt
LIMIT
	-- A null limit means "no limit", so 0 means return all
	NULLIF(@limit_opt :: int, 0);

-- name: UpdateUserStatus :one
UPDATE
	users
SET
	status = $2,
	updated_at = $3
WHERE
	id = $1 RETURNING *;


-- name: GetAuthorizationUserRoles :one
-- This function returns roles for authorization purposes. Implied member roles
-- are included.
SELECT
	-- username is returned just to help for logging purposes
	-- status is used to enforce 'suspended' users, as all roles are ignored
	--	when suspended.
	id, username, status,
	array_cat(
		-- All users are members
			array_append(users.rbac_roles, 'member'),
		-- All org_members get the org-member role for their orgs
			array_append(organization_members.roles, 'organization-member:'||organization_members.organization_id::text)) :: text[]
		AS roles
FROM
	users
LEFT JOIN organization_members
	ON id = user_id
WHERE
	id = @user_id;
