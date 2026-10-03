-- Restore the previous telemetry_locks event_type constraint. Existing
-- user_secrets_summary rows must be removed first or the new constraint
-- check would fail.
DELETE FROM telemetry_locks WHERE event_type = 'user_secrets_summary';

ALTER TABLE telemetry_locks DROP CONSTRAINT telemetry_lock_event_type_constraint;
ALTER TABLE telemetry_locks ADD CONSTRAINT telemetry_lock_event_type_constraint
    CHECK (event_type IN ('aibridge_interceptions_summary', 'boundary_usage_summary'));
