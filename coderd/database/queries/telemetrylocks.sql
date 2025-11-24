-- name: InsertTelemetryLock :exec
-- Inserts a new lock row into the telemetry_locks table. Replicas should call
-- this function prior to attempting to generate or publish a heartbeat event to
-- the telemetry service.
-- If the query returns a duplicate primary key error, the replica should not
-- attempt to generate or publish the event to the telemetry service.
INSERT INTO
    telemetry_locks (event_type, period_ending_at)
VALUES
    ($1, $2);

-- name: DeleteOldTelemetryLocks :exec
-- Deletes old telemetry locks from the telemetry_locks table.
DELETE FROM
    telemetry_locks
WHERE
    period_ending_at < @period_ending_at_before::timestamptz;
