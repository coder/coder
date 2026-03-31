-- name: GetGroupMembers :many
SELECT * FROM group_members_expanded
WHERE CASE
      WHEN @include_system::bool THEN TRUE
      ELSE
        user_is_system = false
        END;

-- name: GetGroupMembersByGroupID :many
SELECT *
FROM group_members_expanded
WHERE group_id = @group_id
  -- Filter by system type
  AND CASE
      WHEN @include_system::bool THEN TRUE
      ELSE
        user_is_system = false
      END;

-- name: GetGroupMembersByGroupIDPaginated :many
SELECT
	*, COUNT(*) OVER() AS count
FROM
	group_members_expanded
WHERE
	group_members_expanded.group_id = @group_id
	AND CASE
		-- This allows using the last element on a page as effectively a cursor.
		-- This is an important option for scripts that need to paginate without
		-- duplicating or missing data.
		WHEN @after_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN (
			-- The pagination cursor is the last ID of the previous page.
			-- The query is ordered by the username field, so select all
			-- rows after the cursor.
			(LOWER(user_username)) > (
				SELECT
					LOWER(user_username)
				FROM
					group_members_expanded
				WHERE
					group_id = @group_id
					AND user_id = @after_id
			)
		)
		ELSE true
	END
	-- Start filters
	-- Filter by email or username
	AND CASE
		WHEN @search :: text != '' THEN (
			user_email ILIKE concat('%', @search, '%')
			OR user_username ILIKE concat('%', @search, '%')
		)
		ELSE true
	END
	-- Filter by name (display name)
	AND CASE
		WHEN @name :: text != '' THEN
			user_name ILIKE concat('%', @name, '%')
		ELSE true
	END
	-- Filter by status
	AND CASE
		-- @status needs to be a text because it can be empty, If it was
		-- user_status enum, it would not.
		WHEN cardinality(@status :: user_status[]) > 0 THEN
			user_status = ANY(@status :: user_status[])
		ELSE true
	END
	-- Filter by rbac_roles
	AND CASE
		-- @rbac_role allows filtering by rbac roles. If 'member' is included, show everyone, as
		-- everyone is a member.
		WHEN cardinality(@rbac_role :: text[]) > 0 AND 'member' != ANY(@rbac_role :: text[]) THEN
			user_rbac_roles && @rbac_role :: text[]
		ELSE true
	END
	-- Filter by last_seen
	AND CASE
		WHEN @last_seen_before :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			user_last_seen_at <= @last_seen_before
		ELSE true
	END
	AND CASE
		WHEN @last_seen_after :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			user_last_seen_at >= @last_seen_after
		ELSE true
	END
	-- Filter by created_at
	AND CASE
		WHEN @created_before :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			user_created_at <= @created_before
		ELSE true
	END
	AND CASE
		WHEN @created_after :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			user_created_at >= @created_after
		ELSE true
	END
	-- Filter by system type
	AND CASE
		WHEN @include_system::bool THEN TRUE
		ELSE user_is_system = false
	END
	-- Filter by github.com user ID
	AND CASE
		WHEN @github_com_user_id :: bigint != 0 THEN
			user_github_com_user_id = @github_com_user_id
		ELSE true
	END
	-- Filter by login_type
	AND CASE
		WHEN cardinality(@login_type :: login_type[]) > 0 THEN
			user_login_type = ANY(@login_type :: login_type[])
		ELSE true
	END
	-- Filter by service account.
	AND CASE
		WHEN sqlc.narg('is_service_account') :: boolean IS NOT NULL THEN
			user_is_service_account = sqlc.narg('is_service_account') :: boolean
		ELSE true
	END
	-- End of filters
ORDER BY
	-- Deterministic and consistent ordering of all users. This is to ensure consistent pagination.
	LOWER(user_username) ASC OFFSET @offset_opt
LIMIT
	-- A null limit means "no limit", so 0 means return all
	NULLIF(@limit_opt :: int, 0);

-- name: GetGroupMembersCountByGroupID :one
-- Returns the total count of members in a group. Shows the total
-- count even if the caller does not have read access to ResourceGroupMember.
-- They only need ResourceGroup read access.
SELECT COUNT(*)
FROM group_members_expanded
WHERE group_id = @group_id
  -- Filter by system type
  AND CASE
      WHEN @include_system::bool THEN TRUE
      ELSE
        user_is_system = false
        END;

-- InsertUserGroupsByID adds a user to all provided groups, if they exist.
-- name: InsertUserGroupsByID :many
WITH groups AS (
	SELECT
		id
	FROM
		groups
	WHERE
		groups.id = ANY(@group_ids :: uuid [])
)
INSERT INTO
	group_members (user_id, group_id)
SELECT
	@user_id,
	groups.id
FROM
	groups
-- If there is a conflict, the user is already a member
ON CONFLICT DO NOTHING
RETURNING group_id;

-- name: RemoveUserFromGroups :many
DELETE FROM
	group_members
WHERE
	user_id = @user_id AND
	group_id = ANY(@group_ids :: uuid [])
RETURNING group_id;

-- name: InsertGroupMember :exec
INSERT INTO
    group_members (user_id, group_id)
VALUES
    ($1, $2);

-- name: DeleteGroupMemberFromGroup :exec
DELETE FROM
	group_members
WHERE
	user_id = $1 AND
	group_id = $2;
