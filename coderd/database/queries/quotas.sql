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
	(wb.workspace_id) wb.workspace_id,
	wb.daily_cost
FROM
	workspace_builds wb
 -- This INNER JOIN prevents a seq scan of the workspace_builds table.
 -- Limit the rows to the absolute minimum required, which is all workspaces
 -- in a given organization for a given user.
INNER JOIN
	workspaces on wb.workspace_id = workspaces.id
WHERE
	workspaces.owner_id = @owner_id AND
	workspaces.organization_id = @organization_id
ORDER BY
	wb.workspace_id,
	wb.created_at DESC
)
SELECT
	coalesce(SUM(daily_cost), 0)::BIGINT
FROM
	workspaces
INNER JOIN latest_builds ON
	latest_builds.workspace_id = workspaces.id
WHERE
	NOT deleted AND
	-- We can likely remove these conditions since we check above.
	-- But it does not hurt to be defensive and make sure future query changes
	-- do not break anything.
	workspaces.owner_id = @owner_id AND
	workspaces.organization_id = @organization_id
;
