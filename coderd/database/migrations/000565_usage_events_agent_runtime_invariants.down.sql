-- IF EXISTS matches 000561's down migration, so rolling back out of order
-- (e.g. during an incident) is a no-op instead of a failure.
DROP INDEX IF EXISTS idx_usage_events_agent_runtime;
CREATE INDEX idx_usage_events_agent_runtime
  ON usage_events (event_type, created_at)
  WHERE event_type = 'hb_agent_runtime_v1';

ALTER TABLE usage_events
  DROP CONSTRAINT IF EXISTS usage_events_agent_runtime_hour_aligned;
