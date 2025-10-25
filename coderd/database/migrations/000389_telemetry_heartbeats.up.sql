CREATE TABLE telemetry_heartbeats (
    event_type TEXT NOT NULL CONSTRAINT telemetry_heartbeat_event_type_check CHECK (event_type IN ('aibridge_interceptions_snapshot')),
    heartbeat_timestamp TIMESTAMP WITH TIME ZONE NOT NULL,

    PRIMARY KEY (event_type, heartbeat_timestamp)
);

COMMENT ON TABLE telemetry_heartbeats IS 'Telemetry heartbeat tracking table for deduplication of event types across replicas.';
COMMENT ON COLUMN telemetry_heartbeats.event_type IS 'The type of event that was sent.';
COMMENT ON COLUMN telemetry_heartbeats.heartbeat_timestamp IS 'The timestamp of the heartbeat event. Usually the end of the period for which the event contains data.';

CREATE INDEX idx_telemetry_heartbeats_heartbeat_timestamp ON telemetry_heartbeats (heartbeat_timestamp);

-- Automatically clean up old heartbeats on every insert. Since this table
-- doesn't get that much traffic (less than 100 inserts per hour), it should be
-- fairly cheap to just delete old heartbeats on every insert.
--
-- We don't need to persist heartbeat rows for longer than 24 hours, as they are
-- only used for deduplication across replicas. The time needs to be long enough
-- to cover the maximum interval of a heartbeat event (currently 1 hour) plus
-- some buffer.
CREATE OR REPLACE FUNCTION delete_old_telemetry_heartbeats() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM telemetry_heartbeats
    WHERE heartbeat_timestamp < NOW() - INTERVAL '24 hours';
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_delete_old_telemetry_heartbeats
    AFTER INSERT ON telemetry_heartbeats
    FOR EACH ROW
    EXECUTE FUNCTION delete_old_telemetry_heartbeats();
