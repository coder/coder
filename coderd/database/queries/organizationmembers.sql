-- name: OrganizationMembers :many
SELECT
	sqlc.embed(organization_members),
	users.username
FROM
	organization_members
		INNER JOIN
	users ON organization_members.user_id = users.id
WHERE
	true
  -- Filter by organization id
  AND CASE
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
