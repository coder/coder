-- Restore the original telemetry_locks event_type constraint.
ALTER TABLE telemetry_locks DROP CONSTRAINT telemetry_lock_event_type_constraint;
ALTER TABLE telemetry_locks ADD CONSTRAINT telemetry_lock_event_type_constraint
    CHECK (event_type IN ('aibridge_interceptions_summary'));

DROP TABLE boundary_usage_stats;

-- No-op for boundary_usage scopes: keep enum values to avoid dependency churn.
