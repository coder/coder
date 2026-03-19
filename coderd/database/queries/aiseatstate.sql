-- name: GetUserAISeatStates :many
-- Returns user IDs that are consuming an AI seat.
-- A user consumes an AI seat if they have AI bridge interceptions
-- or own a workspace with AI tasks.
SELECT
	user_id
FROM (
	SELECT DISTINCT initiator_id AS user_id FROM aibridge_interceptions
	UNION
	SELECT DISTINCT workspaces.owner_id AS user_id
	FROM workspaces
	INNER JOIN workspace_builds ON workspace_builds.workspace_id = workspaces.id
	WHERE workspace_builds.has_ai_task = true AND workspaces.deleted = false
) ai_users
WHERE
	user_id = ANY(@user_ids::uuid[]);
