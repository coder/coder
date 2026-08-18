-- Nullable columns with no default: metadata-only changes, so no rows are
-- rewritten and no index is built while the ACCESS EXCLUSIVE lock is held.
-- Existing failed rows keep NULL; publish failure detection falls back to
-- inserted_at for them, preserving any active warning across the upgrade.
ALTER TABLE usage_events
    ADD COLUMN first_failed_at timestamp with time zone,
    ADD COLUMN last_failed_at timestamp with time zone;

COMMENT ON COLUMN usage_events.first_failed_at IS 'The time of the first failed publish attempt in the row''s current failure streak. Publish failure detection measures failure age from this timestamp, so failures start the failure threshold from the first failed attempt rather than from insertion.';

COMMENT ON COLUMN usage_events.last_failed_at IS 'The time of the most recent failed publish attempt that left the row unpublished. A failed attempt more than 24 hours after the previous one starts a new failure streak (resets first_failed_at), so failures predating an interval where publishing was disabled do not count toward the failure threshold after re-enablement.';
