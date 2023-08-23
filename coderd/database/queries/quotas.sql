-- name: GetQuotaAllowanceForUser :one
SELECT
	coalesce(SUM(quota_allowance), 0)::BIGINT
FROM
	groups g
LEFT JOIN group_members gm ON
	g.id = gm.group_id
WHERE
	user_id = $1
OR
    g.id = g.organization_id;

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
WHERE NOT deleted AND workspaces.owner_id = $1;
