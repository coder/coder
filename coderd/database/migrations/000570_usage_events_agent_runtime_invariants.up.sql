-- The usage generator writes hb_agent_runtime_v1 rows with created_at at
-- the UTC hourly bucket start and exactly one row per bucket. Uniqueness
-- keeps any consumer that sums runtime_ms from counting a bucket twice;
-- the alignment CHECK protects the attribution model, which charges a
-- bucket to the usage period containing its start.
--
-- Both statements validate existing rows. Every supported writer has always
-- produced conforming data, so a pre-existing violator is anomalous and
-- failing the migration loudly beats silently rewriting usage rows.
ALTER TABLE usage_events
  ADD CONSTRAINT usage_events_agent_runtime_hour_aligned
  CHECK (
    event_type <> 'hb_agent_runtime_v1'
    OR date_trunc('hour', (created_at AT TIME ZONE 'UTC')) = (created_at AT TIME ZONE 'UTC')
  );

-- Inserts keep their (id) arbiter: re-inserting a bucket under its
-- deterministic id stays a silent no-op, while a duplicate bucket row under
-- a different id raises a unique violation (generateBucket in
-- enterprise/coderd/usage/generator.go handles it).
DROP INDEX idx_usage_events_agent_runtime;
CREATE UNIQUE INDEX idx_usage_events_agent_runtime
  ON usage_events (event_type, created_at)
  WHERE event_type = 'hb_agent_runtime_v1';
