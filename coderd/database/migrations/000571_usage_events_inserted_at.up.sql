-- Adding a column with a non-volatile default is a metadata-only change on
-- PostgreSQL 11+, so existing rows are not rewritten; they all report the
-- ALTER's now() as their inserted_at.
--
-- Existing rows are deliberately not backfilled from created_at: unpublished
-- rows are unbounded on deployments whose license disables publishing (the
-- inserter keeps recording while the publisher never selects), and this
-- transaction holds the ALTER's ACCESS EXCLUSIVE lock, so any backfill
-- rewrite could block usage-event writes for the duration of the upgrade.
-- The one-time cost is that events already stuck at upgrade time report
-- inserted_at as the migration time, so a pre-existing publish failure
-- warning clears and only re-fires once the failure threshold elapses again.
ALTER TABLE usage_events
    ADD COLUMN inserted_at timestamp with time zone NOT NULL DEFAULT now();

COMMENT ON COLUMN usage_events.inserted_at IS 'The time the row was inserted into the database. Unlike created_at, this is always the wall-clock insertion time, so backfilled heartbeat events (whose created_at is the historical bucket start) are not misdetected as stuck by publish failure detection.';
