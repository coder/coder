CREATE TABLE telemetry_heartbeats (
    event_type TEXT NOT NULL CONSTRAINT telemetry_heartbeat_event_type_check CHECK (event_type IN ('aibridge_interceptions_summary')),
    heartbeat_timestamp TIMESTAMP WITH TIME ZONE NOT NULL,

    PRIMARY KEY (event_type, heartbeat_timestamp)
);

COMMENT ON TABLE telemetry_heartbeats IS 'Telemetry heartbeat tracking table for deduplication of event types across replicas.';
COMMENT ON COLUMN telemetry_heartbeats.event_type IS 'The type of event that was sent.';
COMMENT ON COLUMN telemetry_heartbeats.heartbeat_timestamp IS 'The timestamp of the heartbeat event. Usually the end of the period for which the event contains data.';

CREATE INDEX idx_telemetry_heartbeats_heartbeat_timestamp ON telemetry_heartbeats (heartbeat_timestamp);
