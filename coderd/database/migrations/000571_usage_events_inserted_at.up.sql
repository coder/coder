-- Adding a column with a non-volatile default is a metadata-only change on
-- PostgreSQL 11+, so existing rows are not rewritten; they all report the
-- ALTER's now() as their inserted_at.
ALTER TABLE usage_events
    ADD COLUMN inserted_at timestamp with time zone NOT NULL DEFAULT now();

-- Backfill from created_at only where publish failure detection reads
-- inserted_at: unpublished rows. These are few by construction (the
-- publisher keeps up in healthy deployments), which keeps this UPDATE from
-- rewriting the whole table while the ALTER's ACCESS EXCLUSIVE lock is held
-- for the rest of the migration transaction. For rows that are genuinely
-- stuck, created_at approximates insertion time well enough to keep an
-- existing failure warning alive across the upgrade.
UPDATE usage_events SET inserted_at = created_at WHERE published_at IS NULL;

COMMENT ON COLUMN usage_events.inserted_at IS 'The time the row was inserted into the database. Unlike created_at, this is always the wall-clock insertion time, so backfilled heartbeat events (whose created_at is the historical bucket start) are not misdetected as stuck by publish failure detection.';
