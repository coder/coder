-- name: UpsertBoundaryUsageStats :one
-- Upserts boundary usage statistics for a replica. All values are replaced with
-- the current in-memory state. Returns true if this was an insert (new period),
-- false if update.
INSERT INTO boundary_usage_stats (
    replica_id,
    unique_workspaces_count,
    unique_users_count,
    allowed_requests,
    denied_requests,
    window_start,
    updated_at
) VALUES (
    @replica_id,
    @unique_workspaces_count,
    @unique_users_count,
    @allowed_requests,
    @denied_requests,
    NOW(),
    NOW()
) ON CONFLICT (replica_id) DO UPDATE SET
    unique_workspaces_count = EXCLUDED.unique_workspaces_count,
    unique_users_count = EXCLUDED.unique_users_count,
    allowed_requests = EXCLUDED.allowed_requests,
    denied_requests = EXCLUDED.denied_requests,
    updated_at = NOW()
RETURNING (xmax = 0) AS new_period;

-- name: GetBoundaryUsageSummary :one
-- Aggregates boundary usage statistics across all replicas. Filters to only
-- include data where window_start is within the given interval to exclude
-- stale data from failed resets.
SELECT
    COALESCE(SUM(unique_workspaces_count), 0)::bigint AS unique_workspaces,
    COALESCE(SUM(unique_users_count), 0)::bigint AS unique_users,
    COALESCE(SUM(allowed_requests), 0)::bigint AS allowed_requests,
    COALESCE(SUM(denied_requests), 0)::bigint AS denied_requests
FROM boundary_usage_stats
WHERE window_start >= NOW() - (@max_staleness_ms::bigint || ' ms')::interval;

-- name: ResetBoundaryUsageStats :exec
-- Deletes all boundary usage statistics. Called after telemetry reports the
-- aggregated stats. Each replica will insert a fresh row on its next flush.
DELETE FROM boundary_usage_stats;

-- name: DeleteBoundaryUsageStatsByReplicaID :exec
-- Deletes boundary usage statistics for a specific replica.
DELETE FROM boundary_usage_stats WHERE replica_id = @replica_id;
