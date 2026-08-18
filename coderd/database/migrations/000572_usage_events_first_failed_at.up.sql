-- Nullable column with no default: a metadata-only change, so no rows are
-- rewritten and no index is built while the ACCESS EXCLUSIVE lock is held.
-- Existing failed rows keep NULL; publish failure detection falls back to
-- inserted_at for them, preserving any active warning across the upgrade.
ALTER TABLE usage_events
    ADD COLUMN first_failed_at timestamp with time zone;

COMMENT ON COLUMN usage_events.first_failed_at IS 'The time of the first failed publish attempt that left the row unpublished. Publish failure detection measures failure age from this timestamp, so a backlog accumulated while publishing was disabled warns only after the failure threshold elapses from the first post-(re-)enablement failure rather than from insertion.';
