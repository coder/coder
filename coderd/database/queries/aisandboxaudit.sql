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

-- name: GetAISandboxSessionsByAIAgentID :many
-- Confinement sessions naming one AI agent. Reads the attribution snapshot
-- directly rather than resolving through a workspace, so a session found here
-- survives the cleanup of anything but itself.
SELECT * FROM ai_sandbox_sessions
WHERE ai_agent_id = @ai_agent_id
ORDER BY started_at DESC;

-- name: ListAISandboxSessionsBySponsor :many
-- Lists confinement sessions attributed to a sponsoring user whose start or
-- end falls inside the (@after_time, @before_time) window, newest activity
-- first. Zero time bounds disable that bound.
SELECT * FROM ai_sandbox_sessions
WHERE sponsor_user_id = @sponsor_user_id
	AND CASE
		WHEN @ai_agent_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN ai_agent_id = @ai_agent_id::uuid
		ELSE true
	END
	AND (
		(
			(@after_time::timestamptz = '0001-01-01 00:00:00+00'::timestamptz OR started_at > @after_time::timestamptz)
			AND (@before_time::timestamptz = '0001-01-01 00:00:00+00'::timestamptz OR started_at < @before_time::timestamptz)
		)
		OR (
			ended_at IS NOT NULL
			AND (@after_time::timestamptz = '0001-01-01 00:00:00+00'::timestamptz OR ended_at > @after_time::timestamptz)
			AND (@before_time::timestamptz = '0001-01-01 00:00:00+00'::timestamptz OR ended_at < @before_time::timestamptz)
		)
	)
ORDER BY GREATEST(started_at, COALESCE(ended_at, started_at)) DESC
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 100);

-- name: ListAISandboxNetworkEventAggregatesBySponsor :many
-- Aggregates egress decisions per (session, host, action) bucket for the
-- sponsor timeline. Raw events stay behind the per-session drill-down.
-- occurred_at aggregates to the newest occurrence in the bucket; protocol
-- and port snapshot that newest event. The session join recovers the
-- workspace for drill-down links and is LEFT so events survive session
-- purges.
SELECT
	e.session_id,
	e.host,
	e.action,
	e.ai_agent_id,
	s.workspace_id AS workspace_id,
	MAX(e.occurred_at)::timestamptz AS last_occurred_at,
	COUNT(*)::bigint AS event_count,
	(array_agg(e.protocol ORDER BY e.occurred_at DESC, e.id DESC))[1]::text AS protocol,
	(array_agg(e.port ORDER BY e.occurred_at DESC, e.id DESC))[1]::integer AS port
FROM ai_sandbox_network_events e
LEFT JOIN ai_sandbox_sessions s ON s.id = e.session_id
WHERE e.sponsor_user_id = @sponsor_user_id
	AND CASE
		WHEN @ai_agent_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN e.ai_agent_id = @ai_agent_id::uuid
		ELSE true
	END
	AND (@after_time::timestamptz = '0001-01-01 00:00:00+00'::timestamptz OR e.occurred_at > @after_time::timestamptz)
	AND (@before_time::timestamptz = '0001-01-01 00:00:00+00'::timestamptz OR e.occurred_at < @before_time::timestamptz)
GROUP BY e.session_id, e.host, e.action, e.ai_agent_id, s.workspace_id
ORDER BY last_occurred_at DESC
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 100);

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

-- name: GetAISandboxNetworkEventsByAIAgentIDPaged :many
-- Egress decisions naming one AI agent, oldest first, paged by row id.
--
-- Filters on the event's own attribution snapshot rather than joining through
-- ai_sandbox_sessions. The snapshot is copied onto every event server side for
-- exactly this reason: an event must remain attributable after its session is
-- gone.
SELECT * FROM ai_sandbox_network_events
WHERE ai_agent_id = @ai_agent_id
  AND id > @after_id
ORDER BY id ASC
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
