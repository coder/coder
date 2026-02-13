-- name: FindOrCreateSessionForDisconnect :one
-- Find existing session within time window, or create new one.
-- Uses advisory lock to prevent duplicate sessions from concurrent disconnects.
-- Groups by client_hostname (with IP fallback) to match the live session
-- grouping in mergeWorkspaceConnectionsIntoSessions.
WITH lock AS (
    SELECT pg_advisory_xact_lock(
        hashtext(@workspace_id::text || COALESCE(@client_hostname, host(@ip::inet), 'unknown'))
    )
),
existing AS (
    SELECT id FROM workspace_sessions
    WHERE workspace_id = @workspace_id::uuid
      AND (
          (@client_hostname IS NOT NULL AND client_hostname = @client_hostname)
          OR
          (@client_hostname IS NULL AND client_hostname IS NULL AND ip = @ip::inet)
      )
      AND @connect_time BETWEEN started_at - INTERVAL '30 minutes' AND ended_at + INTERVAL '30 minutes'
    ORDER BY started_at DESC
    LIMIT 1
),
new_session AS (
    INSERT INTO workspace_sessions (workspace_id, agent_id, ip, client_hostname, short_description, started_at, ended_at)
    SELECT @workspace_id::uuid, @agent_id, @ip::inet, @client_hostname, @short_description, @connect_time, @disconnect_time
    WHERE NOT EXISTS (SELECT 1 FROM existing)
    RETURNING id
),
updated_session AS (
    UPDATE workspace_sessions
    SET started_at = LEAST(started_at, @connect_time),
        ended_at = GREATEST(ended_at, @disconnect_time)
    WHERE id = (SELECT id FROM existing)
    RETURNING id
)
SELECT COALESCE(
    (SELECT id FROM updated_session),
    (SELECT id FROM new_session)
) AS id;

-- name: GetWorkspaceSessionsOffset :many
SELECT
    ws.*,
    (SELECT COUNT(*) FROM connection_logs cl WHERE cl.session_id = ws.id) AS connection_count
FROM workspace_sessions ws
WHERE ws.workspace_id = @workspace_id
  AND CASE WHEN @started_after::timestamptz != '0001-01-01 00:00:00Z'::timestamptz
      THEN ws.started_at >= @started_after ELSE true END
  AND CASE WHEN @started_before::timestamptz != '0001-01-01 00:00:00Z'::timestamptz
      THEN ws.started_at <= @started_before ELSE true END
ORDER BY ws.started_at DESC
LIMIT @limit_count
OFFSET @offset_count;

-- name: CountWorkspaceSessions :one
SELECT COUNT(*) FROM workspace_sessions ws
WHERE ws.workspace_id = @workspace_id
  AND CASE WHEN @started_after::timestamptz != '0001-01-01 00:00:00Z'::timestamptz
      THEN ws.started_at >= @started_after ELSE true END
  AND CASE WHEN @started_before::timestamptz != '0001-01-01 00:00:00Z'::timestamptz
      THEN ws.started_at <= @started_before ELSE true END;

-- name: GetConnectionLogsBySessionIDs :many
SELECT * FROM connection_logs
WHERE session_id = ANY(@session_ids::uuid[])
ORDER BY session_id, connect_time DESC;

-- name: GetConnectionLogByConnectionID :one
SELECT * FROM connection_logs
WHERE connection_id = @connection_id
  AND workspace_id = @workspace_id
  AND agent_name = @agent_name
LIMIT 1;

-- name: GetGlobalWorkspaceSessionsOffset :many
SELECT
    ws.*,
    w.name AS workspace_name,
    workspace_owner.username AS workspace_owner_username,
    (SELECT COUNT(*) FROM connection_logs cl WHERE cl.session_id = ws.id) AS connection_count
FROM workspace_sessions ws
JOIN workspaces w ON w.id = ws.workspace_id
JOIN users workspace_owner ON workspace_owner.id = w.owner_id
WHERE
  CASE WHEN @workspace_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid
    THEN ws.workspace_id = @workspace_id ELSE true END
  AND CASE WHEN @workspace_owner::text != ''
    THEN workspace_owner.username = @workspace_owner ELSE true END
  AND CASE WHEN @started_after::timestamptz != '0001-01-01 00:00:00Z'::timestamptz
    THEN ws.started_at >= @started_after ELSE true END
  AND CASE WHEN @started_before::timestamptz != '0001-01-01 00:00:00Z'::timestamptz
    THEN ws.started_at <= @started_before ELSE true END
ORDER BY ws.started_at DESC
LIMIT @limit_count
OFFSET @offset_count;

-- name: CountGlobalWorkspaceSessions :one
SELECT COUNT(*) FROM workspace_sessions ws
JOIN workspaces w ON w.id = ws.workspace_id
JOIN users workspace_owner ON workspace_owner.id = w.owner_id
WHERE
  CASE WHEN @workspace_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid
    THEN ws.workspace_id = @workspace_id ELSE true END
  AND CASE WHEN @workspace_owner::text != ''
    THEN workspace_owner.username = @workspace_owner ELSE true END
  AND CASE WHEN @started_after::timestamptz != '0001-01-01 00:00:00Z'::timestamptz
    THEN ws.started_at >= @started_after ELSE true END
  AND CASE WHEN @started_before::timestamptz != '0001-01-01 00:00:00Z'::timestamptz
    THEN ws.started_at <= @started_before ELSE true END;

