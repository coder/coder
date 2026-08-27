-- Sponsor-scoped access paths for the AI audit timeline. Each source is
-- filtered by sponsor and windowed by time, newest first.
CREATE INDEX idx_aibridge_interceptions_sponsor_started_at
	ON aibridge_interceptions (sponsor_user_id, started_at DESC)
	WHERE sponsor_user_id IS NOT NULL;

CREATE INDEX idx_ai_sandbox_sessions_sponsor_started_at
	ON ai_sandbox_sessions (sponsor_user_id, started_at DESC);

CREATE INDEX idx_ai_sandbox_network_events_sponsor_occurred_at
	ON ai_sandbox_network_events (sponsor_user_id, occurred_at DESC);
