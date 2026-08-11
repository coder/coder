-- name: UpsertAISandboxSession :one
-- Creates or idempotently re-asserts a confinement session. The reporting
-- supervisor generates the ID; on conflict only ended_at may change, so
-- attribution snapshots are immutable once recorded.
INSERT INTO ai_sandbox_sessions (
	id,
	workspace_id,
	reporter_agent_id,
	confined_agent_id,
	ai_agent_id,
	sponsor_user_id,
	egress_enforcement,
	started_at,
	ended_at,
	created_at
) VALUES (
	@id,
	@workspace_id,
	@reporter_agent_id,
	@confined_agent_id,
	@ai_agent_id,
	@sponsor_user_id,
	@egress_enforcement,
	@started_at,
	@ended_at,
	@created_at
)
ON CONFLICT (id) DO UPDATE SET
	ended_at = EXCLUDED.ended_at
RETURNING *;

-- name: GetAISandboxSessionByID :one
SELECT * FROM ai_sandbox_sessions WHERE id = @id;

-- name: GetAISandboxSessionsByWorkspaceID :many
SELECT * FROM ai_sandbox_sessions
WHERE workspace_id = @workspace_id
ORDER BY started_at DESC;

-- name: InsertAISandboxNetworkEvents :execrows
-- Batch-inserts egress policy decisions. Attribution snapshots are copied
-- server-side from the owning session row onto every event.
INSERT INTO ai_sandbox_network_events (
	session_id,
	occurred_at,
	protocol,
	host,
	port,
	action,
	policy_revision,
	ai_agent_id,
	sponsor_user_id,
	created_at
)
SELECT
	unnest(@session_id :: uuid[]),
	unnest(@occurred_at :: timestamptz[]),
	unnest(@protocol :: text[]),
	unnest(@host :: text[]),
	unnest(@port :: int[]),
	unnest(@action :: text[]),
	unnest(@policy_revision :: bigint[]),
	unnest(@ai_agent_id :: uuid[]),
	unnest(@sponsor_user_id :: uuid[]),
	unnest(@created_at :: timestamptz[]);

-- name: GetAISandboxNetworkEventsBySessionID :many
SELECT * FROM ai_sandbox_network_events
WHERE session_id = @session_id
ORDER BY occurred_at ASC, id ASC;

-- name: GetAISandboxNetworkEventsBySessionIDPaged :many
SELECT e.* FROM ai_sandbox_network_events e
WHERE e.session_id = @session_id
  AND e.id > @after_id
  AND EXISTS (
      SELECT 1
      FROM ai_sandbox_sessions s
      WHERE s.id = e.session_id
        AND s.workspace_id = @workspace_id
  )
ORDER BY e.id ASC
LIMIT @limit_count;

-- name: DeleteOldAISandboxNetworkEvents :execrows
-- Deletes egress audit events older than the given time, bounded by a row
-- limit to avoid long-running transactions.
WITH old_events AS (
	SELECT id
	FROM ai_sandbox_network_events
	WHERE occurred_at < @before_time::timestamptz
	ORDER BY occurred_at ASC
	LIMIT @limit_count
)
DELETE FROM ai_sandbox_network_events
USING old_events
WHERE ai_sandbox_network_events.id = old_events.id;

-- name: DeleteOldAISandboxSessions :execrows
-- Deletes confinement sessions that ended before the given time and no
-- longer have any retained events.
WITH old_sessions AS (
	SELECT s.id
	FROM ai_sandbox_sessions s
	WHERE s.ended_at IS NOT NULL
	  AND s.ended_at < @before_time::timestamptz
	  AND NOT EXISTS (
	      SELECT 1 FROM ai_sandbox_network_events e WHERE e.session_id = s.id
	  )
	ORDER BY s.ended_at ASC
	LIMIT @limit_count
)
DELETE FROM ai_sandbox_sessions
USING old_sessions
WHERE ai_sandbox_sessions.id = old_sessions.id;
