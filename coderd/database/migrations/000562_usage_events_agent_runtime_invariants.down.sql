DROP INDEX idx_usage_events_agent_runtime;
CREATE INDEX idx_usage_events_agent_runtime
  ON usage_events (event_type, created_at)
  WHERE event_type = 'hb_agent_runtime_v1';

ALTER TABLE usage_events
  DROP CONSTRAINT usage_events_agent_runtime_hour_aligned;
