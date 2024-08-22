-- name: GetQuotaAllowanceForUser :one
SELECT
	coalesce(SUM(groups.quota_allowance), 0)::BIGINT
FROM
	(
		-- Select all groups this user is a member of. This will also include
		-- the "Everyone" group for organizations the user is a member of.
		SELECT * FROM group_members_expanded
		         WHERE
		             @user_id = user_id AND
		             @organization_id = group_members_expanded.organization_id
	) AS members
INNER JOIN groups ON
	members.group_id = groups.id
;

-- name: GetQuotaConsumedForUser :one
WITH latest_builds AS (
SELECT
	DISTINCT ON
	(workspace_id) id,
	workspace_id,
	daily_cost
FROM
	workspace_builds wb
ORDER BY
	workspace_id,
	created_at DESC
)
SELECT
	coalesce(SUM(daily_cost), 0)::BIGINT
FROM
	workspaces
JOIN latest_builds ON
	latest_builds.workspace_id = workspaces.id
WHERE NOT
	deleted AND
	workspaces.owner_id = @owner_id AND
	workspaces.organization_id = @organization_id
;
