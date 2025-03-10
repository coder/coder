-- name: OrganizationMembers :many
-- Arguments are optional with uuid.Nil to ignore.
--  - Use just 'organization_id' to get all members of an org
--  - Use just 'user_id' to get all orgs a user is a member of
--  - Use both to get a specific org member row
SELECT
	sqlc.embed(organization_members),
	users.username, users.avatar_url, users.name, users.email, users.rbac_roles as "global_roles"
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
	COUNT(*) OVER() AS count
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
ORDER BY
	-- Deterministic and consistent ordering of all users. This is to ensure consistent pagination.
	LOWER(username) ASC OFFSET @offset_opt
LIMIT
	-- A null limit means "no limit", so 0 means return all
	NULLIF(@limit_opt :: int, 0);
