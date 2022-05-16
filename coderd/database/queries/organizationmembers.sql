-- name: GetOrganizationMemberByUserID :one
SELECT
	*
FROM
	organization_members
WHERE
	organization_id = $1
	AND user_id = $2
LIMIT
	1;

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


-- name: GetOrganizationMembershipsByUserID :many
SELECT
	*
FROM
	organization_members
WHERE
  user_id = $1;

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
