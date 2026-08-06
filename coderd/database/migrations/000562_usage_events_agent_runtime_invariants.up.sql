-- The entitlements read path (GetTotalUsageHBAgentRuntimeV1) sums
-- hb_agent_runtime_v1 rows between exact timestamps and depends on two
-- invariants the usage generator provides but nothing enforced: created_at
-- is always the UTC hourly bucket start, and there is exactly one row per
-- bucket. Enforce both so a misaligned or duplicate row fails loudly at
-- insert time instead of silently skewing a customer-facing usage figure.
ALTER TABLE usage_events
  ADD CONSTRAINT usage_events_agent_runtime_hour_aligned
  CHECK (
    event_type <> 'hb_agent_runtime_v1'
    OR date_trunc('hour', (created_at AT TIME ZONE 'UTC')) = (created_at AT TIME ZONE 'UTC')
  );

-- Replace the non-unique partial index with a unique one of the same shape:
-- same columns and predicate, so reads are served identically. The
-- generator's ON CONFLICT (id) DO NOTHING does not match this index, so a
-- duplicate bucket row under a different id surfaces as an insert error and
-- gets logged rather than being counted twice.
DROP INDEX idx_usage_events_agent_runtime;
CREATE UNIQUE INDEX idx_usage_events_agent_runtime
  ON usage_events (event_type, created_at)
  WHERE event_type = 'hb_agent_runtime_v1';
