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

-- name: SetTransactionLockTimeout :exec
-- Bounds how long later lock waits in the current transaction may block.
-- A wait longer than lock_timeout_ms fails with lock_not_available
-- instead of blocking. The setting reverts when the transaction ends.
SELECT set_config('lock_timeout', format('%sms', @lock_timeout_ms::bigint), true);
