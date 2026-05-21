-- name: AcquireLock :exec
-- Blocks until the lock is acquired.
--
-- This must be called from within a transaction. The lock will be automatically
-- released when the transaction ends.
SELECT pg_advisory_xact_lock($1);

-- name: TryAcquireLock :one
-- Non blocking lock. Returns true if the lock was acquired, false otherwise.
--
-- This must be called from within a transaction. The lock will be automatically
-- released when the transaction ends.
SELECT pg_try_advisory_xact_lock($1);
