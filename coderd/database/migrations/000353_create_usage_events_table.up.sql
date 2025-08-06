CREATE TYPE usage_event_type AS ENUM (
  'dc_managed_agents_v1'
);

COMMENT ON TYPE usage_event_type IS 'The usage event type with version. "dc" means "discrete" (e.g. a single event, for counters), "hb" means "heartbeat" (e.g. a recurring event that contains a total count of usage generated from the database, for gauges).';

CREATE TABLE usage_events (
  id TEXT PRIMARY KEY,
  event_type usage_event_type NOT NULL,
  event_data JSONB NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE NOT NULL,
  publish_started_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
  published_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
  failure_message TEXT DEFAULT NULL
);

COMMENT ON TABLE usage_events IS 'usage_events contains usage data that is collected from the product and potentially shipped to the usage collector service.';
COMMENT ON COLUMN usage_events.id IS 'For "discrete" event types, this is a random UUID. For "heartbeat" event types, this is a combination of the event type and a truncated timestamp.';
COMMENT ON COLUMN usage_events.event_data IS 'Event payload. Determined by the matching usage struct for this event type.';
COMMENT ON COLUMN usage_events.publish_started_at IS 'Set to a timestamp while the event is being published by a Coder replica to the usage collector service. Used to avoid duplicate publishes by multiple replicas. Timestamps older than 1 hour are considered expired.';
COMMENT ON COLUMN usage_events.published_at IS 'Set to a timestamp when the event is successfully (or permanently unsuccessfully) published to the usage collector service. If set, the event should never be attempted to be published again.';
COMMENT ON COLUMN usage_events.failure_message IS 'Set to an error message when the event is temporarily or permanently unsuccessfully published to the usage collector service.';

CREATE INDEX idx_usage_events_created_at ON usage_events (created_at);
CREATE INDEX idx_usage_events_publish_started_at ON usage_events (publish_started_at);
CREATE INDEX idx_usage_events_published_at ON usage_events (published_at);
