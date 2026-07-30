CREATE TABLE usage_events (
  id TEXT PRIMARY KEY,
  -- We use a TEXT column with a CHECK constraint rather than an enum because of
  -- the limitations with adding new values to an enum and using them in the
  -- same transaction.
  event_type TEXT NOT NULL CONSTRAINT usage_event_type_check CHECK (event_type IN ('dc_managed_agents_v1')),
  event_data JSONB NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE NOT NULL,
  publish_started_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
  published_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
  failure_message TEXT DEFAULT NULL
);

COMMENT ON TABLE usage_events IS 'usage_events contains usage data that is collected from the product and potentially shipped to the usage collector service.';
COMMENT ON COLUMN usage_events.id IS 'For "discrete" event types, this is a random UUID. For "heartbeat" event types, this is a combination of the event type and a truncated timestamp.';
COMMENT ON COLUMN usage_events.event_type IS 'The usage event type with version. "dc" means "discrete" (e.g. a single event, for counters), "hb" means "heartbeat" (e.g. a recurring event that contains a total count of usage generated from the database, for gauges).';
COMMENT ON COLUMN usage_events.event_data IS 'Event payload. Determined by the matching usage struct for this event type.';
COMMENT ON COLUMN usage_events.publish_started_at IS 'Set to a timestamp while the event is being published by a Coder replica to the usage collector service. Used to avoid duplicate publishes by multiple replicas. Timestamps older than 1 hour are considered expired.';
COMMENT ON COLUMN usage_events.published_at IS 'Set to a timestamp when the event is successfully (or permanently unsuccessfully) published to the usage collector service. If set, the event should never be attempted to be published again.';
COMMENT ON COLUMN usage_events.failure_message IS 'Set to an error message when the event is temporarily or permanently unsuccessfully published to the usage collector service.';

-- Create an index with all three fields used by the
-- SelectUsageEventsForPublishing query.
CREATE INDEX idx_usage_events_select_for_publishing
  ON usage_events (published_at, publish_started_at, created_at);
