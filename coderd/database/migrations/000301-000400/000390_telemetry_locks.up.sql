CREATE TABLE telemetry_locks (
    event_type TEXT NOT NULL CONSTRAINT telemetry_lock_event_type_constraint CHECK (event_type IN ('aibridge_interceptions_summary')),
    period_ending_at TIMESTAMP WITH TIME ZONE NOT NULL,

    PRIMARY KEY (event_type, period_ending_at)
);

COMMENT ON TABLE telemetry_locks IS 'Telemetry lock tracking table for deduplication of heartbeat events across replicas.';
COMMENT ON COLUMN telemetry_locks.event_type IS 'The type of event that was sent.';
COMMENT ON COLUMN telemetry_locks.period_ending_at IS 'The heartbeat period end timestamp.';

CREATE INDEX idx_telemetry_locks_period_ending_at ON telemetry_locks (period_ending_at);
