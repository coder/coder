-- The ai_audit_trail RBAC resource needs its low-level API key scope enum
-- values. The scope stays internal-only: an AI agent's own credential must
-- not read its owner's trail.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_audit_trail:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_audit_trail:read';

-- The AI audit trail lists egress records by sponsoring user within a time
-- window. The journals already carry subject indexes; the activity logs
-- need sponsor-scoped ones.
CREATE INDEX idx_ai_sandbox_sessions_sponsor_started
	ON ai_sandbox_sessions (sponsor_user_id, started_at);

CREATE INDEX idx_ai_sandbox_network_events_sponsor_occurred
	ON ai_sandbox_network_events (sponsor_user_id, occurred_at);
