-- name: GetQuotaAllowanceForUser :one
SELECT
	SUM(quota_allowance)
FROM
	group_members gm
JOIN groups g ON
	g.id = gm.group_id
WHERE
	user_id = $1;

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
	SUM(daily_cost)
FROM
	workspaces
JOIN latest_builds ON
	latest_builds.workspace_id = workspaces.id
WHERE NOT deleted AND workspaces.owner_id = $1;
