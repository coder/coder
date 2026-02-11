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
			(@status = 'completed' AND disconnect_time IS NOT NULL))
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
			(@status = 'completed' AND disconnect_time IS NOT NULL))
		ELSE true
	END
	-- Authorize Filter clause will be injected below in
	-- CountAuthorizedConnectionLogs
	-- @authorize_filter
;

-- name: DeleteOldConnectionLogs :execrows
WITH old_logs AS (
	SELECT id
	FROM connection_logs
	WHERE connect_time < @before_time::timestamp with time zone
	ORDER BY connect_time ASC
	LIMIT @limit_count
)
DELETE FROM connection_logs
USING old_logs
WHERE connection_logs.id = old_logs.id;

-- name: UpsertConnectionLog :one
INSERT INTO connection_logs (
	id,
	connect_time,
	organization_id,
	workspace_owner_id,
	workspace_id,
	workspace_name,
	agent_name,
	agent_id,
	type,
	code,
	ip,
	user_agent,
	user_id,
	slug_or_port,
	connection_id,
	disconnect_reason,
	disconnect_time,
	updated_at,
	session_id,
	client_hostname,
	short_description
) VALUES
	($1, @time, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
	-- If we've only received a disconnect event, mark the event as immediately
	-- closed.
	 CASE
		 WHEN @connection_status::connection_status = 'disconnected'
		 THEN @time :: timestamp with time zone
		 ELSE NULL
	 END,
	 @time, $16, $17, $18)
ON CONFLICT (connection_id, workspace_id, agent_name)
DO UPDATE SET
	updated_at = @time,
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
	END,
	agent_id = COALESCE(connection_logs.agent_id, EXCLUDED.agent_id)
RETURNING *;


-- name: CloseOpenAgentConnectionLogsForWorkspace :execrows
UPDATE connection_logs
SET
	disconnect_time = GREATEST(connect_time, @closed_at :: timestamp with time zone),
	-- Do not overwrite any existing reason.
	disconnect_reason = COALESCE(disconnect_reason, @reason :: text)
WHERE disconnect_time IS NULL
	AND workspace_id = @workspace_id :: uuid
	AND type = ANY(@types :: connection_type[]);

-- name: GetOngoingAgentConnectionsLast24h :many
WITH ranked AS (
	SELECT
		id,
		connect_time,
		organization_id,
		workspace_owner_id,
		workspace_id,
		workspace_name,
		agent_name,
		type,
		ip,
		code,
		user_agent,
		user_id,
		slug_or_port,
		connection_id,
		disconnect_time,
		disconnect_reason,
		agent_id,
		updated_at,
		session_id,
		client_hostname,
		short_description,
		row_number() OVER (
			PARTITION BY workspace_id, agent_name
			ORDER BY connect_time DESC
		) AS rn
	FROM
		connection_logs
	WHERE
		workspace_id = ANY(@workspace_ids :: uuid[])
		AND agent_name = ANY(@agent_names :: text[])
		AND type = ANY(@types :: connection_type[])
		AND disconnect_time IS NULL
		AND (
			-- Non-web types always included while connected.
			type NOT IN ('workspace_app', 'port_forwarding')
			-- Agent-reported web connections have NULL user_agent
			-- and carry proper disconnect lifecycle tracking.
			OR user_agent IS NULL
			-- Proxy-reported web connections (non-NULL user_agent)
			-- rely on updated_at being bumped on each token refresh.
			OR updated_at >= @app_active_since :: timestamp with time zone
		)
		AND connect_time >= @since :: timestamp with time zone
)
SELECT
	id,
	connect_time,
	organization_id,
	workspace_owner_id,
	workspace_id,
	workspace_name,
	agent_name,
	type,
	ip,
	code,
	user_agent,
	user_id,
	slug_or_port,
	connection_id,
	disconnect_time,
	disconnect_reason,
	updated_at,
	session_id,
	client_hostname,
	short_description
FROM
	ranked
WHERE
	sqlc.arg('rn')::bigint <= @per_agent_limit
ORDER BY
	workspace_id,
	agent_name,
	connect_time DESC;

-- name: CloseConnectionLogsAndCreateSessions :execrows
-- Atomically closes open connections and creates sessions grouping by IP.
-- Used when a workspace is stopped/deleted.
WITH connections_to_close AS (
    SELECT id, ip, connect_time, agent_id, client_hostname, short_description
    FROM connection_logs
    WHERE disconnect_time IS NULL
      AND connection_logs.workspace_id = @workspace_id
      AND type = ANY(@types::connection_type[])
),
session_groups AS (
    SELECT 
        ip,
        MIN(connect_time) AS started_at,
        @closed_at::timestamptz AS ended_at,
        (array_agg(agent_id ORDER BY connect_time))[1] AS agent_id,
        (array_agg(client_hostname ORDER BY connect_time) FILTER (WHERE client_hostname IS NOT NULL))[1] AS client_hostname,
        (array_agg(short_description ORDER BY connect_time) FILTER (WHERE short_description IS NOT NULL))[1] AS short_description
    FROM connections_to_close
    GROUP BY ip
),
new_sessions AS (
    INSERT INTO workspace_sessions (workspace_id, agent_id, ip, client_hostname, short_description, started_at, ended_at)
    SELECT @workspace_id, agent_id, ip, client_hostname, short_description, started_at, ended_at
    FROM session_groups
    RETURNING id, ip
)
UPDATE connection_logs cl
SET 
    disconnect_time = @closed_at,
    disconnect_reason = COALESCE(disconnect_reason, @reason),
    session_id = ns.id
FROM connections_to_close ctc
JOIN new_sessions ns ON ctc.ip = ns.ip
WHERE cl.id = ctc.id;

