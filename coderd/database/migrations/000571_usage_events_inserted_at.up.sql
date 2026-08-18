ALTER TABLE usage_events
    ADD COLUMN inserted_at timestamp with time zone NOT NULL DEFAULT now();

-- Existing rows predate the column, so approximate their insertion time with
-- the usage timestamp. The two match for all rows except heartbeat events
-- backfilled after downtime, which are rare.
UPDATE usage_events SET inserted_at = created_at;

COMMENT ON COLUMN usage_events.inserted_at IS 'The time the row was inserted into the database. Unlike created_at, this is always the wall-clock insertion time, so backfilled heartbeat events (whose created_at is the historical bucket start) are not misdetected as stuck by publish failure detection.';
