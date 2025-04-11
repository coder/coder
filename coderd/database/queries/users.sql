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
	id = @user_id
	AND NOT is_system
RETURNING *;

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
	deleted = false
  	AND CASE WHEN @include_system::bool THEN TRUE ELSE is_system = false END;

-- name: GetActiveUserCount :one
SELECT
	COUNT(*)
FROM
	users
WHERE
	status = 'active'::user_status AND deleted = false
	AND CASE WHEN @include_system::bool THEN TRUE ELSE is_system = false END;

-- name: InsertUser :one
INSERT INTO
	users (
		id,
		email,
		username,
		name,
		hashed_password,
		created_at,
		updated_at,
		rbac_roles,
		login_type,
		status
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9,
		-- if the status passed in is empty, fallback to dormant, which is what
		-- we were doing before.
		COALESCE(NULLIF(@status::text, '')::user_status, 'dormant'::user_status)
	) RETURNING *;

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

-- name: UpdateUserGithubComUserID :exec
UPDATE
	users
SET
	github_com_user_id = $2
WHERE
	id = $1;

-- name: GetUserThemePreference :one
SELECT
	value as theme_preference
FROM
	user_configs
WHERE
	user_id = @user_id
	AND key = 'theme_preference';

-- name: UpdateUserThemePreference :one
INSERT INTO
	user_configs (user_id, key, value)
VALUES
	(@user_id, 'theme_preference', @theme_preference)
ON CONFLICT
	ON CONSTRAINT user_configs_pkey
DO UPDATE
SET
	value = @theme_preference
WHERE user_configs.user_id = @user_id
	AND user_configs.key = 'theme_preference'
RETURNING *;

-- name: GetUserTerminalFont :one
SELECT
	value as terminal_font
FROM
	user_configs
WHERE
	user_id = @user_id
	AND key = 'terminal_font';

-- name: UpdateUserTerminalFont :one
INSERT INTO
	user_configs (user_id, key, value)
VALUES
	(@user_id, 'terminal_font', @terminal_font)
ON CONFLICT
	ON CONSTRAINT user_configs_pkey
DO UPDATE
SET
	value = @terminal_font
WHERE user_configs.user_id = @user_id
	AND user_configs.key = 'terminal_font'
RETURNING *;

-- name: GetUserTerminalFontSize :one
SELECT
	value as terminal_font_size
FROM
	user_configs
WHERE
	user_id = @user_id
	AND key = 'terminal_font_size';

-- name: UpdateUserTerminalFontSize :one
INSERT INTO
	user_configs (user_id, key, value)
VALUES
	(@user_id, 'terminal_font_size', @terminal_font_size)
ON CONFLICT
	ON CONSTRAINT user_configs_pkey
DO UPDATE
SET
	value = @terminal_font_size
WHERE user_configs.user_id = @user_id
	AND user_configs.key = 'terminal_font_size'
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
	hashed_password = $2,
	hashed_one_time_passcode = NULL,
	one_time_passcode_expires_at = NULL
WHERE
	id = $1;

-- name: UpdateUserDeletedByID :exec
UPDATE
	users
SET
	deleted = true
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
	-- Filter by created_at
	AND CASE
		WHEN @created_before :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			created_at <= @created_before
		ELSE true
	END
	AND CASE
		WHEN @created_after :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			created_at >= @created_after
		ELSE true
	END
  	AND CASE
  	    WHEN @include_system::bool THEN TRUE
  	    ELSE
			is_system = false
	END
	AND CASE
		WHEN @github_com_user_id :: bigint != 0 THEN
			github_com_user_id = @github_com_user_id
		ELSE true
	END
	-- Filter by login_type
	AND CASE
		WHEN cardinality(@login_type :: login_type[]) > 0 THEN
			login_type = ANY(@login_type :: login_type[])
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
				-- The roles are returned as a flat array, org scoped and site side.
				-- Concatenating the organization id scopes the organization roles.
				array_agg(org_roles || ':' || organization_members.organization_id::text)
			FROM
				organization_members,
				-- All org_members get the organization-member role for their orgs
				unnest(
					array_append(roles, 'organization-member')
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
		AND NOT is_system
RETURNING id, email, username, last_seen_at;

-- AllUserIDs returns all UserIDs regardless of user status or deletion.
-- name: AllUserIDs :many
SELECT DISTINCT id FROM USERS
	WHERE CASE WHEN @include_system::bool THEN TRUE ELSE is_system = false END;

-- name: UpdateUserHashedOneTimePasscode :exec
UPDATE
    users
SET
    hashed_one_time_passcode = $2,
    one_time_passcode_expires_at = $3
WHERE
    id = $1
;
