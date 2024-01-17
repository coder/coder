-- name: UpdateUserLoginType :one
UPDATE
	users
SET
	login_type = @new_login_type,
	hashed_password = CASE WHEN @new_login_type = 'password' :: login_type THEN
		users.hashed_password
	ELSE
		-- If the login type is not password, then the password should be
        -- cleared.
		'':: bytea
	END
WHERE
	id = @user_id RETURNING *;

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
-- This shouldn't check for deleted, because it's frequently used
-- to look up references to actions. eg. a user could build a workspace
-- for another user, then be deleted... we still want them to appear!
SELECT * FROM users WHERE id = ANY(@ids :: uuid [ ]);

-- name: GetUserByEmailOrUsername :one
SELECT
	*
FROM
	users
WHERE
	(LOWER(username) = LOWER(@username) OR LOWER(email) = LOWER(@email)) AND
	deleted = false
LIMIT
	1;

-- name: GetUserCount :one
SELECT
	COUNT(*)
FROM
	users
WHERE
	deleted = false;

-- name: GetActiveUserCount :one
SELECT
	COUNT(*)
FROM
	users
WHERE
	status = 'active'::user_status AND deleted = false;

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
	updated_at = $5,
	name = $6
WHERE
	id = $1
RETURNING *;

-- name: UpdateUserAppearanceSettings :one
UPDATE
	users
SET
	theme_preference = $2,
	updated_at = $3
WHERE
	id = $1
RETURNING *;

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

-- name: UpdateUserDeletedByID :exec
UPDATE
	users
SET
	deleted = $2
WHERE
	id = $1;

-- name: GetUsers :many
-- This will never return deleted users.
SELECT
	*, COUNT(*) OVER() AS count
FROM
	users
WHERE
	users.deleted = false
	AND CASE
		-- This allows using the last element on a page as effectively a cursor.
		-- This is an important option for scripts that need to paginate without
		-- duplicating or missing data.
		WHEN @after_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN (
			-- The pagination cursor is the last ID of the previous page.
			-- The query is ordered by the username field, so select all
			-- rows after the cursor.
			(LOWER(username)) > (
				SELECT
					LOWER(username)
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
	-- Filter by last_seen
	AND CASE
		WHEN @last_seen_before :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			last_seen_at <= @last_seen_before
		ELSE true
	END
	AND CASE
		WHEN @last_seen_after :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			last_seen_at >= @last_seen_after
		ELSE true
	END
	-- End of filters

	-- Authorize Filter clause will be injected below in GetAuthorizedUsers
	-- @authorize_filter
ORDER BY
	-- Deterministic and consistent ordering of all users. This is to ensure consistent pagination.
	LOWER(username) ASC OFFSET @offset_opt
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

-- name: UpdateUserLastSeenAt :one
UPDATE
	users
SET
	last_seen_at = $2,
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
	-- All user roles, including their org roles.
	array_cat(
		-- All users are members
		array_append(users.rbac_roles, 'member'),
		(
			SELECT
				array_agg(org_roles)
			FROM
				organization_members,
				-- All org_members get the org-member role for their orgs
				unnest(
					array_append(roles, 'organization-member:' || organization_members.organization_id::text)
				) AS org_roles
			WHERE
				user_id = users.id
		)
	) :: text[] AS roles,
	-- All groups the user is in.
	(
		SELECT
			array_agg(
				group_members.group_id :: text
			)
		FROM
			group_members
		WHERE
			user_id = users.id
	) :: text[] AS groups
FROM
	users
WHERE
	id = @user_id;

-- name: UpdateUserQuietHoursSchedule :one
UPDATE
	users
SET
	quiet_hours_schedule = $2
WHERE
	id = $1
RETURNING *;


-- name: UpdateInactiveUsersToDormant :many
UPDATE
    users
SET
    status = 'dormant'::user_status,
	updated_at = @updated_at
WHERE
    last_seen_at < @last_seen_after :: timestamp
    AND status = 'active'::user_status
RETURNING id, email, last_seen_at;

-- AllUserIDs returns all UserIDs regardless of user status or deletion.
-- name: AllUserIDs :many
SELECT DISTINCT id FROM USERS;
