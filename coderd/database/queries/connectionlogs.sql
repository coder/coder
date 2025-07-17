-- name: GetConnectionLogsOffset :many
SELECT
	sqlc.embed(connection_logs),
	-- sqlc.embed(users) would be nice but it does not seem to play well with
	-- left joins. This user metadata is necessary for parity with the audit logs
	-- API.
	users.username AS user_username,
	users.name AS user_name,
	users.email AS user_email,
	users.created_at AS user_created_at,
	users.updated_at AS user_updated_at,
	users.last_seen_at AS user_last_seen_at,
	users.status AS user_status,
	users.login_type AS user_login_type,
	users.rbac_roles AS user_roles,
	users.avatar_url AS user_avatar_url,
	users.deleted AS user_deleted,
	users.quiet_hours_schedule AS user_quiet_hours_schedule,
	workspace_owner.username AS workspace_owner_username,
	organizations.name AS organization_name,
	organizations.display_name AS organization_display_name,
	organizations.icon AS organization_icon
FROM
	connection_logs
JOIN users AS workspace_owner ON
	connection_logs.workspace_owner_id = workspace_owner.id
LEFT JOIN users ON
	connection_logs.user_id = users.id
JOIN organizations ON
	connection_logs.organization_id = organizations.id
WHERE
	-- Filter organization_id
	CASE
		WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			connection_logs.organization_id = @organization_id
		ELSE true
	END
	-- Filter by workspace owner username
	AND CASE
		WHEN @workspace_owner :: text != '' THEN
			workspace_owner_id = (
				SELECT id FROM users
				WHERE lower(username) = lower(@workspace_owner) AND deleted = false
			)
		ELSE true
	END
	-- Filter by workspace_owner_id
	AND CASE
		WHEN @workspace_owner_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			workspace_owner_id = @workspace_owner_id
		ELSE true
	END
	-- Filter by workspace_owner_email
	AND CASE
		WHEN @workspace_owner_email :: text != '' THEN
			workspace_owner_id = (
				SELECT id FROM users
				WHERE email = @workspace_owner_email AND deleted = false
			)
		ELSE true
	END
	-- Filter by type
	AND CASE
		WHEN @type :: text != '' THEN
			type = @type :: connection_type
		ELSE true
	END
	-- Filter by user_id
	AND CASE
		WHEN @user_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			user_id = @user_id
		ELSE true
	END
	-- Filter by username
	AND CASE
		WHEN @username :: text != '' THEN
			user_id = (
				SELECT id FROM users
				WHERE lower(username) = lower(@username) AND deleted = false
			)
		ELSE true
	END
	-- Filter by user_email
	AND CASE
		WHEN @user_email :: text != '' THEN
			users.email = @user_email
		ELSE true
	END
	-- Filter by connected_after
	AND CASE
		WHEN @connected_after :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			connect_time >= @connected_after
		ELSE true
	END
	-- Filter by connected_before
	AND CASE
		WHEN @connected_before :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			connect_time <= @connected_before
		ELSE true
	END
	-- Filter by workspace_id
	AND CASE
		WHEN @workspace_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			connection_logs.workspace_id = @workspace_id
		ELSE true
	END
	-- Filter by connection_id
	AND CASE
		WHEN @connection_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			connection_logs.connection_id = @connection_id
		ELSE true
	END
	-- Filter by whether the session has a disconnect_time
	AND CASE
		WHEN @status :: text != '' THEN
			((@status = 'ongoing' AND disconnect_time IS NULL) OR
			(@status = 'completed' AND disconnect_time IS NOT NULL)) AND
			-- Exclude web events, since we don't know their close time.
			"type" NOT IN ('workspace_app', 'port_forwarding')
		ELSE true
	END
	-- Authorize Filter clause will be injected below in
	-- GetAuthorizedConnectionLogsOffset
	-- @authorize_filter
ORDER BY
	connect_time DESC
LIMIT
	-- a limit of 0 means "no limit". The connection log table is unbounded
	-- in size, and is expected to be quite large. Implement a default
	-- limit of 100 to prevent accidental excessively large queries.
	COALESCE(NULLIF(@limit_opt :: int, 0), 100)
OFFSET
	@offset_opt;

-- name: CountConnectionLogs :one
SELECT
	COUNT(*) AS count
FROM
	connection_logs
JOIN users AS workspace_owner ON
	connection_logs.workspace_owner_id = workspace_owner.id
LEFT JOIN users ON
	connection_logs.user_id = users.id
JOIN organizations ON
	connection_logs.organization_id = organizations.id
WHERE
	-- Filter organization_id
	CASE
		WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			connection_logs.organization_id = @organization_id
		ELSE true
	END
	-- Filter by workspace owner username
	AND CASE
		WHEN @workspace_owner :: text != '' THEN
			workspace_owner_id = (
				SELECT id FROM users
				WHERE lower(username) = lower(@workspace_owner) AND deleted = false
			)
		ELSE true
	END
	-- Filter by workspace_owner_id
	AND CASE
		WHEN @workspace_owner_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			workspace_owner_id = @workspace_owner_id
		ELSE true
	END
	-- Filter by workspace_owner_email
	AND CASE
		WHEN @workspace_owner_email :: text != '' THEN
			workspace_owner_id = (
				SELECT id FROM users
				WHERE email = @workspace_owner_email AND deleted = false
			)
		ELSE true
	END
	-- Filter by type
	AND CASE
		WHEN @type :: text != '' THEN
			type = @type :: connection_type
		ELSE true
	END
	-- Filter by user_id
	AND CASE
		WHEN @user_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			user_id = @user_id
		ELSE true
	END
	-- Filter by username
	AND CASE
		WHEN @username :: text != '' THEN
			user_id = (
				SELECT id FROM users
				WHERE lower(username) = lower(@username) AND deleted = false
			)
		ELSE true
	END
	-- Filter by user_email
	AND CASE
		WHEN @user_email :: text != '' THEN
			users.email = @user_email
		ELSE true
	END
	-- Filter by connected_after
	AND CASE
		WHEN @connected_after :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			connect_time >= @connected_after
		ELSE true
	END
	-- Filter by connected_before
	AND CASE
		WHEN @connected_before :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			connect_time <= @connected_before
		ELSE true
	END
	-- Filter by workspace_id
	AND CASE
		WHEN @workspace_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			connection_logs.workspace_id = @workspace_id
		ELSE true
	END
	-- Filter by connection_id
	AND CASE
		WHEN @connection_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			connection_logs.connection_id = @connection_id
		ELSE true
	END
	-- Filter by whether the session has a disconnect_time
	AND CASE
		WHEN @status :: text != '' THEN
			((@status = 'ongoing' AND disconnect_time IS NULL) OR
			(@status = 'completed' AND disconnect_time IS NOT NULL)) AND
			-- Exclude web events, since we don't know their close time.
			"type" NOT IN ('workspace_app', 'port_forwarding')
		ELSE true
	END
	-- Authorize Filter clause will be injected below in
	-- CountAuthorizedConnectionLogs
	-- @authorize_filter
;

-- name: UpsertConnectionLog :one
INSERT INTO connection_logs (
	id,
	connect_time,
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
	disconnect_reason,
	disconnect_time
) VALUES
	($1, @time, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
	-- If we've only received a disconnect event, mark the event as immediately
	-- closed.
	 CASE
		 WHEN @connection_status::connection_status = 'disconnected'
		 THEN @time :: timestamp with time zone
		 ELSE NULL
	 END)
ON CONFLICT (connection_id, workspace_id, agent_name)
DO UPDATE SET
	-- No-op if the connection is still open.
	disconnect_time = CASE
		WHEN @connection_status::connection_status = 'disconnected'
		-- Can only be set once
		AND connection_logs.disconnect_time IS NULL
		THEN EXCLUDED.connect_time
		ELSE connection_logs.disconnect_time
	END,
	disconnect_reason = CASE
		WHEN @connection_status::connection_status = 'disconnected'
		-- Can only be set once
		AND connection_logs.disconnect_reason IS NULL
		THEN EXCLUDED.disconnect_reason
		ELSE connection_logs.disconnect_reason
	END,
	code = CASE
		WHEN @connection_status::connection_status = 'disconnected'
		-- Can only be set once
		AND connection_logs.code IS NULL
		THEN EXCLUDED.code
		ELSE connection_logs.code
	END
RETURNING *;
