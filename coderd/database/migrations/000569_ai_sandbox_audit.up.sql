CREATE TABLE ai_sandbox_sessions (
	-- The reporting supervisor generates the ID so events can reference
	-- the session before the create round-trip completes and retries are
	-- idempotent.
	id uuid NOT NULL PRIMARY KEY,
	-- The columns below are intentionally not foreign keys: egress audit
	-- history must survive identity revocation, workspace deletion, and
	-- identity cleanup.
	workspace_id uuid NOT NULL,
	reporter_agent_id uuid NOT NULL,
	confined_agent_id uuid NOT NULL,
	ai_agent_id uuid NOT NULL,
	sponsor_user_id uuid NOT NULL,
	egress_enforcement text NOT NULL CHECK (egress_enforcement IN ('forced', 'advisory', 'none')),
	started_at timestamptz NOT NULL,
	ended_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE ai_sandbox_sessions IS
	'Confinement sessions reported by the supervisor-owned egress proxy for AI-bound execution. Attribution columns are server-resolved snapshots without foreign keys so audit history survives identity cleanup.';
COMMENT ON COLUMN ai_sandbox_sessions.reporter_agent_id IS
	'Workspace agent that owns the egress proxy and reported this session. Not a foreign key; retained after agent deletion.';
COMMENT ON COLUMN ai_sandbox_sessions.confined_agent_id IS
	'AI-bound workspace agent being confined: equals reporter_agent_id for an AI-designated workspace, or the sandboxed child agent. Not a foreign key; retained after agent deletion.';
COMMENT ON COLUMN ai_sandbox_sessions.ai_agent_id IS
	'AI agent identity snapshot. Not a foreign key to ai_agents; retained after identity revocation and cleanup.';
COMMENT ON COLUMN ai_sandbox_sessions.sponsor_user_id IS
	'Sponsoring human user snapshot. Not a foreign key to users; retained after user cleanup.';
COMMENT ON COLUMN ai_sandbox_sessions.egress_enforcement IS
	'Admin attestation of routing coverage (forced, advisory, or none). Recorded, not verified.';

CREATE INDEX idx_ai_sandbox_sessions_ai_agent_id ON ai_sandbox_sessions (ai_agent_id);
CREATE INDEX idx_ai_sandbox_sessions_started_at ON ai_sandbox_sessions (started_at);

CREATE TABLE ai_sandbox_network_events (
	id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	-- Intentionally not a foreign key to ai_sandbox_sessions: transient
	-- session insert failures must not lose events, and retention may
	-- delete sessions and events independently.
	session_id uuid NOT NULL,
	occurred_at timestamptz NOT NULL,
	protocol text NOT NULL CHECK (protocol IN ('connect', 'http', 'sni', 'tcp')),
	host text NOT NULL,
	port integer NOT NULL,
	action text NOT NULL CHECK (action IN ('allowed', 'denied')),
	policy_revision bigint NOT NULL DEFAULT 0,
	-- Attribution snapshots are denormalized onto every event (copied
	-- server-side from the session row) so each retained record remains
	-- attributable without any surviving session or identity row.
	ai_agent_id uuid NOT NULL,
	sponsor_user_id uuid NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE ai_sandbox_network_events IS
	'Egress policy decisions observed by the supervisor-owned proxy for AI-bound execution. Attribution columns are server-resolved snapshots without foreign keys so audit history survives identity cleanup.';
COMMENT ON COLUMN ai_sandbox_network_events.session_id IS
	'Owning ai_sandbox_sessions.id. Not a foreign key; events must outlive session and identity cleanup.';
COMMENT ON COLUMN ai_sandbox_network_events.policy_revision IS
	'Egress policy revision that produced the decision, or 0 while the supervisor runs the bootstrap deny-all fallback.';

CREATE INDEX idx_ai_sandbox_network_events_session_id ON ai_sandbox_network_events (session_id);
CREATE INDEX idx_ai_sandbox_network_events_occurred_at ON ai_sandbox_network_events (occurred_at);
