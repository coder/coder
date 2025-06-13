-- name: GetConnectionLogsOffset :many
SELECT
	sqlc.embed(connection_logs),
	users.username AS user_username,
	workspace_owner.username AS workspace_owner_username,
	COUNT(connection_logs.*) OVER () AS count
FROM
	connection_logs
LEFT JOIN users ON
	connection_logs.user_id = users.id
LEFT JOIN users as workspace_owner ON
	connection_logs.workspace_owner_id = workspace_owner.id
WHERE TRUE
	-- Authorize Filter clause will be injected below in
	-- GetAuthorizedConnectionLogsOffset
	-- @authorize_filter
ORDER BY
	"time" DESC
LIMIT
	-- a limit of 0 means "no limit". The connection log table is unbounded
	-- in size, and is expected to be quite large. Implement a default
	-- limit of 100 to prevent accidental excessively large queries.
	COALESCE(NULLIF(@limit_opt :: int, 0), 100)
OFFSET
	@offset_opt;


-- name: UpsertConnectionLog :one
INSERT INTO connection_logs (
	id,
	"time",
	organization_id,
	workspace_owner_id,
	workspace_id,
	workspace_name,
	agent_name,
	type,
	code,
	ip,
	user_agent,
	user_id,
	slug_or_port,
	connection_id,
	close_reason
) VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (connection_id, workspace_id, agent_name)
DO UPDATE SET
	-- No-op if the connection is still open.
    close_time = CASE
        WHEN @connection_action::connection_action = 'disconnect'
        THEN EXCLUDED."time"
        ELSE connection_logs.close_time
    END,
    close_reason = CASE
        WHEN @connection_action::connection_action = 'disconnect'
        THEN EXCLUDED.close_reason
        ELSE connection_logs.close_reason
    END
RETURNING *;
