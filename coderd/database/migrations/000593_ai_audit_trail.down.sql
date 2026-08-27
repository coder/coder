-- Enum additions to api_key_scope are intentionally not reverted because
-- Postgres cannot drop enum values safely.
DROP INDEX idx_ai_sandbox_network_events_sponsor_occurred;

DROP INDEX idx_ai_sandbox_sessions_sponsor_started;
