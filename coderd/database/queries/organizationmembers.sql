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
