-- The usage generator writes hb_agent_runtime_v1 rows with two invariants
-- nothing enforced: created_at is always the UTC hourly bucket start, and
-- there is exactly one row per bucket. The entitlements read path
-- (GetTotalUsageHBAgentRuntimeV1) depends on them differently: uniqueness is
-- what keeps SUM from counting a bucket twice, while hour alignment protects
-- the attribution model, which charges a bucket to the usage period
-- containing its start. That rule is only meaningful if bucket starts are
-- where the generator says they are; the SUM itself would add a misaligned
-- timestamp just fine.
--
-- Both statements below validate existing rows, so a pre-existing violator
-- aborts this migration and the upgrade. That is deliberate: every supported
-- writer has always produced aligned, single-row-per-bucket data, so a
-- violator is anomalous (a manual insert or an external tool), and failing
-- loudly is preferable to silently deleting or rewriting usage rows in a
-- migration.
ALTER TABLE usage_events
  ADD CONSTRAINT usage_events_agent_runtime_hour_aligned
  CHECK (
    event_type <> 'hb_agent_runtime_v1'
    OR date_trunc('hour', (created_at AT TIME ZONE 'UTC')) = (created_at AT TIME ZONE 'UTC')
  );

-- Replace the non-unique partial index with a unique one of the same shape:
-- same columns and predicate, so reads are served identically. Inserts keep
-- their (id) arbiter, so re-inserting a bucket under its deterministic id
-- stays a silent no-op, while a duplicate bucket row under a different id
-- (which only a non-generator writer can produce) raises a unique violation
-- instead of being counted twice. Concurrent same-id inserts can surface
-- that violation too; generateBucket in enterprise/coderd/usage/generator.go
-- owns the description of that race and resolves it.
DROP INDEX idx_usage_events_agent_runtime;
CREATE UNIQUE INDEX idx_usage_events_agent_runtime
  ON usage_events (event_type, created_at)
  WHERE event_type = 'hb_agent_runtime_v1';
