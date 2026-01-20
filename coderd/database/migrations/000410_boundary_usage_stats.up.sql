CREATE TABLE boundary_usage_stats (
    replica_id UUID PRIMARY KEY,
    unique_workspaces_count BIGINT NOT NULL DEFAULT 0,
    unique_users_count BIGINT NOT NULL DEFAULT 0,
    allowed_requests BIGINT NOT NULL DEFAULT 0,
    denied_requests BIGINT NOT NULL DEFAULT 0,
    window_start TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE boundary_usage_stats IS 'Per-replica boundary usage statistics for telemetry aggregation.';
COMMENT ON COLUMN boundary_usage_stats.replica_id IS 'The unique identifier of the replica reporting stats.';
COMMENT ON COLUMN boundary_usage_stats.unique_workspaces_count IS 'Count of unique workspaces that used boundary on this replica.';
COMMENT ON COLUMN boundary_usage_stats.unique_users_count IS 'Count of unique users that used boundary on this replica.';
COMMENT ON COLUMN boundary_usage_stats.allowed_requests IS 'Total allowed requests through boundary on this replica.';
COMMENT ON COLUMN boundary_usage_stats.denied_requests IS 'Total denied requests through boundary on this replica.';
COMMENT ON COLUMN boundary_usage_stats.window_start IS 'Start of the time window for these stats, set on first flush after reset.';
COMMENT ON COLUMN boundary_usage_stats.updated_at IS 'Timestamp of the last update to this row.';

-- Add boundary_usage_summary to the telemetry_locks event_type constraint.
ALTER TABLE telemetry_locks DROP CONSTRAINT telemetry_lock_event_type_constraint;
ALTER TABLE telemetry_locks ADD CONSTRAINT telemetry_lock_event_type_constraint
    CHECK (event_type IN ('aibridge_interceptions_summary', 'boundary_usage_summary'));
