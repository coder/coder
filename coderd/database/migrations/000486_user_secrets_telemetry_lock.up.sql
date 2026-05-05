-- Add user_secrets_summary to the telemetry_locks event_type constraint.
-- User secrets aggregates do not have a natural per-row UUID for the
-- telemetry server to dedupe on, so we elect a single replica per
-- snapshot period to report them via this lock table.
ALTER TABLE telemetry_locks DROP CONSTRAINT telemetry_lock_event_type_constraint;
ALTER TABLE telemetry_locks ADD CONSTRAINT telemetry_lock_event_type_constraint
    CHECK (event_type IN ('aibridge_interceptions_summary', 'boundary_usage_summary', 'user_secrets_summary'));
