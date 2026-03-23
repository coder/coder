-- name: OrganizationMembers :many
-- Arguments are optional with uuid.Nil to ignore.
--  - Use just 'organization_id' to get all members of an org
--  - Use just 'user_id' to get all orgs a user is a member of
--  - Use both to get a specific org member row
SELECT
	sqlc.embed(organization_members),
	users.username, users.avatar_url, users.name, users.email, users.rbac_roles as "global_roles",
	users.last_seen_at, users.status, users.login_type,
	users.created_at as user_created_at, users.updated_at as user_updated_at
FROM
	organization_members
		INNER JOIN
	users ON organization_members.user_id = users.id AND users.deleted = false
WHERE
	-- Filter by organization id
	CASE
		WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			organization_id = @organization_id
		ELSE true
	END
	-- Filter by user id
	AND CASE
		WHEN @user_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			user_id = @user_id
		ELSE true
	END
  -- Filter by system type
  	AND CASE
		  WHEN @include_system::bool THEN TRUE
		  ELSE
			  is_system = false
	END
  -- Filter by github user ID. Note that this requires a join on the users table.
  AND CASE
    WHEN @github_user_id :: bigint != 0 THEN
      users.github_com_user_id = @github_user_id
    ELSE true
  END;

-- name: InsertOrganizationMember :one
INSERT INTO
	organization_members (
		organization_id,
		user_id,
		created_at,
		updated_at,
		roles
	)
VALUES
	($1, $2, $3, $4, $5) RETURNING *;

-- name: DeleteOrganizationMember :exec
DELETE
	FROM
		organization_members
	WHERE
		organization_id = @organization_id AND
		user_id = @user_id
;


-- name: GetOrganizationIDsByMemberIDs :many
SELECT
    user_id, array_agg(organization_id) :: uuid [ ] AS "organization_IDs"
FROM
    organization_members
WHERE
    user_id = ANY(@ids :: uuid [ ])
GROUP BY
    user_id;

-- name: UpdateMemberRoles :one
UPDATE
	organization_members
SET
	-- Remove all duplicates from the roles.
	roles = ARRAY(SELECT DISTINCT UNNEST(@granted_roles :: text[]))
WHERE
	user_id = @user_id
	AND organization_id = @org_id
RETURNING *;

-- name: PaginatedOrganizationMembers :many
SELECT
	sqlc.embed(organization_members),
	users.username, users.avatar_url, users.name, users.email, users.rbac_roles as "global_roles",
	users.last_seen_at, users.status, users.login_type,
	users.created_at as user_created_at, users.updated_at as user_updated_at,
	COUNT(*) OVER() AS count
FROM
	organization_members
INNER JOIN
	users ON organization_members.user_id = users.id AND users.deleted = false
WHERE
	CASE
		-- This allows using the last element on a page as effectively a cursor.
		-- This is an important option for scripts that need to paginate without
		-- duplicating or missing data.
		WHEN @after_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN (
			-- The pagination cursor is the last ID of the previous page.
			-- The query is ordered by the username field, so select all
			-- rows after the cursor.
			(LOWER(users.username)) > (
				SELECT
					LOWER(users.username)
				FROM
					organization_members
				INNER JOIN
					users ON organization_members.user_id = users.id
				WHERE
					organization_members.user_id = @after_id
			)
		)
		ELSE true
	END
	-- Start filters
	-- Filter by organization id
	AND CASE
		WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			organization_id = @organization_id
		ELSE true
	END
	-- Filter by email or username
	AND CASE
		WHEN @search :: text != '' THEN (
			users.email ILIKE concat('%', @search, '%')
			OR users.username ILIKE concat('%', @search, '%')
		)
		ELSE true
	END
	-- Filter by name (display name)
	AND CASE
		WHEN @name :: text != '' THEN
			users.name ILIKE concat('%', @name, '%')
		ELSE true
	END
	-- Filter by status
	AND CASE
		-- @status needs to be a text because it can be empty, If it was
		-- user_status enum, it would not.
		WHEN cardinality(@status :: user_status[]) > 0 THEN
			users.status = ANY(@status :: user_status[])
		ELSE true
	END
	-- Filter by global rbac_roles
	AND CASE
		-- @rbac_role allows filtering by rbac roles. If 'member' is included, show everyone, as
		-- everyone is a member.
		WHEN cardinality(@rbac_role :: text[]) > 0 AND 'member' != ANY(@rbac_role :: text[]) THEN
			users.rbac_roles && @rbac_role :: text[]
		ELSE true
	END
	-- Filter by last_seen
	AND CASE
		WHEN @last_seen_before :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			users.last_seen_at <= @last_seen_before
		ELSE true
	END
	AND CASE
		WHEN @last_seen_after :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			users.last_seen_at >= @last_seen_after
		ELSE true
	END
	-- Filter by created_at (user creation date, not date added to org)
	AND CASE
		WHEN @created_before :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			users.created_at <= @created_before
		ELSE true
	END
	AND CASE
		WHEN @created_after :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			users.created_at >= @created_after
		ELSE true
	END
	 -- Filter by system type
	AND CASE
		WHEN @include_system::bool THEN TRUE
		ELSE users.is_system = false
	END
	 -- Filter by github.com user ID
	AND CASE
		WHEN @github_com_user_id :: bigint != 0 THEN
			users.github_com_user_id = @github_com_user_id
		ELSE true
	END
	-- Filter by login_type
	AND CASE
		WHEN cardinality(@login_type :: login_type[]) > 0 THEN
			users.login_type = ANY(@login_type :: login_type[])
		ELSE true
	END
	-- End of filters
ORDER BY
	-- Deterministic and consistent ordering of all users. This is to ensure consistent pagination.
	LOWER(users.username) ASC OFFSET @offset_opt
LIMIT
	-- A null limit means "no limit", so 0 means return all
	NULLIF(@limit_opt :: int, 0);
