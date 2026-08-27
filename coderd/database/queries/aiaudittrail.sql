-- The AI audit trail assembles owner-scoped events from the entity
-- lifecycle journals, the credential use journal, and the egress activity
-- logs. The ai_agent_ledger resolves current ownership (owner_type 'user'
-- in v1); the journals supply the history. Time bounds are exclusive and
-- use the zero-time sentinel convention. Every query takes @owner_id so
-- authorization can bind to the owner being read.

-- name: ListAIAgentLifecycleTrailEvents :many
SELECT
	j.entry_id,
	j.recording_date,
	j.effective_date,
	j.actor_type,
	j.actor,
	j.event,
	j.subject AS ai_agent_id,
	c.creation_site_type,
	c.creation_site_id
FROM ai_agent_lifecycle_journal j
JOIN ai_agent_ledger l ON l.id = j.subject
LEFT JOIN ai_agent_lifecycle_journal_create c ON c.entry_id = j.entry_id
WHERE l.owner_type = 'user'
	AND l.owner_id = @owner_id::uuid
	AND CASE
		WHEN @ai_agent_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN j.subject = @ai_agent_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @after_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN j.effective_date > @after_time::timestamptz
		ELSE true
	END
	AND CASE
		WHEN @before_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN j.effective_date < @before_time::timestamptz
		ELSE true
	END
ORDER BY j.effective_date DESC, j.entry_id DESC
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 100);

-- name: ListAuthorizationLifecycleTrailEvents :many
-- Journal lines carry the event and subject; the actor and dates live on
-- line 0 of the entry, so every line joins back to it.
SELECT
	jl.entry_id,
	jl.line,
	j0.recording_date,
	j0.effective_date,
	j0.actor_type,
	j0.actor,
	jl.event,
	jl.subject AS authorization_id,
	al.principal_type,
	al.principal_id,
	al.agent_id AS ai_agent_id
FROM authorization_lifecycle_journal jl
JOIN authorization_lifecycle_journal j0 ON j0.entry_id = jl.entry_id AND j0.line = 0
JOIN authorization_ledger al ON al.id = jl.subject
JOIN ai_agent_ledger l ON l.id = al.agent_id
WHERE al.agent_type = 'ai_agent'
	AND l.owner_type = 'user'
	AND l.owner_id = @owner_id::uuid
	AND CASE
		WHEN @ai_agent_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN al.agent_id = @ai_agent_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @after_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN j0.effective_date > @after_time::timestamptz
		ELSE true
	END
	AND CASE
		WHEN @before_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN j0.effective_date < @before_time::timestamptz
		ELSE true
	END
ORDER BY j0.effective_date DESC, jl.entry_id DESC, jl.line ASC
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 100);

-- name: ListCredentialLifecycleTrailEvents :many
-- One row per credential a journal entry acts on. The api_key line table
-- contributes the token name where the line describes an api_key issuance.
SELECT
	cl.entry_id,
	cl.line,
	e.recording_date,
	e.effective_date,
	e.actor_type,
	e.actor,
	cl.event,
	cl.subject AS credential_id,
	cr.credential_type,
	cr.holder_id AS ai_agent_id,
	ak.token_name
FROM credential_lifecycle_journal_line cl
JOIN credential_lifecycle_journal e ON e.entry_id = cl.entry_id
JOIN credential_ledger cr ON cr.id = cl.subject
JOIN ai_agent_ledger l ON l.id = cr.holder_id
LEFT JOIN credential_lifecycle_journal_api_key ak ON ak.entry_id = cl.entry_id AND ak.line = cl.line
WHERE cr.holder_type = 'ai_agent'
	AND l.owner_type = 'user'
	AND l.owner_id = @owner_id::uuid
	AND CASE
		WHEN @ai_agent_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN cr.holder_id = @ai_agent_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @after_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN e.effective_date > @after_time::timestamptz
		ELSE true
	END
	AND CASE
		WHEN @before_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN e.effective_date < @before_time::timestamptz
		ELSE true
	END
ORDER BY e.effective_date DESC, cl.entry_id DESC, cl.line ASC
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 100);

-- name: ListCredentialUseTrailEvents :many
-- Presentations are individual events, accepted and refused alike. The
-- actor is the verifier, the party the presentation was made to.
SELECT
	u.entry_id,
	u.recording_date,
	u.effective_date,
	u.actor_type,
	u.actor,
	u.event,
	u.subject AS credential_id,
	u.annotation_source,
	cr.credential_type,
	cr.holder_id AS ai_agent_id
FROM credential_use_journal u
JOIN credential_ledger cr ON cr.id = u.subject
JOIN ai_agent_ledger l ON l.id = cr.holder_id
WHERE cr.holder_type = 'ai_agent'
	AND l.owner_type = 'user'
	AND l.owner_id = @owner_id::uuid
	AND CASE
		WHEN @ai_agent_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN cr.holder_id = @ai_agent_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @after_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN u.effective_date > @after_time::timestamptz
		ELSE true
	END
	AND CASE
		WHEN @before_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN u.effective_date < @before_time::timestamptz
		ELSE true
	END
ORDER BY u.effective_date DESC, u.entry_id DESC
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 100);

-- name: ListAISandboxSessionTrailRows :many
-- Sessions are activity log rows carrying sponsor snapshots; one row can
-- yield a started and an ended event, so the window test is a superset
-- match and the handler trims per event.
SELECT *
FROM ai_sandbox_sessions s
WHERE s.sponsor_user_id = @owner_id::uuid
	AND CASE
		WHEN @ai_agent_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN s.ai_agent_id = @ai_agent_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @after_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN COALESCE(s.ended_at, s.started_at) > @after_time::timestamptz
		ELSE true
	END
	AND CASE
		WHEN @before_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN s.started_at < @before_time::timestamptz
		ELSE true
	END
ORDER BY s.started_at DESC
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 100);

-- name: ListAISandboxEgressTrailAggregates :many
-- Egress decisions aggregated per (session, host, action) bucket within
-- the window. Raw events stay behind the per-session drill-down.
SELECT
	e.session_id,
	e.ai_agent_id,
	e.host,
	e.action,
	MAX(e.occurred_at)::timestamptz AS occurred_at,
	MAX(e.created_at)::timestamptz AS recorded_at,
	COUNT(*)::bigint AS event_count,
	((array_agg(e.protocol ORDER BY e.occurred_at DESC))[1])::text AS protocol,
	((array_agg(e.port ORDER BY e.occurred_at DESC))[1])::integer AS port
FROM ai_sandbox_network_events e
WHERE e.sponsor_user_id = @owner_id::uuid
	AND CASE
		WHEN @ai_agent_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN e.ai_agent_id = @ai_agent_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @after_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN e.occurred_at > @after_time::timestamptz
		ELSE true
	END
	AND CASE
		WHEN @before_time::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN e.occurred_at < @before_time::timestamptz
		ELSE true
	END
GROUP BY e.session_id, e.ai_agent_id, e.host, e.action
ORDER BY MAX(e.occurred_at) DESC
LIMIT COALESCE(NULLIF(@limit_::integer, 0), 100);
