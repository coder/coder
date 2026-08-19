-- Tracks usage events whose most recent publish attempt left them
-- unpublished, keyed by event ID. The publisher maintains rows atomically
-- with each batch's post-publish update, and publish failure detection
-- reads the oldest insertion time instead of scanning the unpublished
-- backlog of usage_events for failed rows. A dedicated relation (rather
-- than indexing the potentially huge usage_events table, which this
-- migration deliberately avoids touching) keeps every maintenance
-- operation index-served and bounded regardless of how many failures
-- accumulate during an outage.
CREATE TABLE usage_events_publish_failures (
    event_id text PRIMARY KEY REFERENCES usage_events (id) ON DELETE CASCADE,
    -- Mirrors usage_events.inserted_at: the event's effective stuck time,
    -- which publish failure detection measures failure age from.
    inserted_at timestamp with time zone NOT NULL,
    -- Mirrors usage_events.created_at: events past the publisher's 30-day
    -- selection window are never published, so their failure rows are
    -- pruned by this column.
    created_at timestamp with time zone NOT NULL
);

COMMENT ON TABLE usage_events_publish_failures IS 'Usage events whose most recent publish attempt left them unpublished. Maintained by the publisher with each batch outcome; read by publish failure detection to find the oldest failing event without scanning the usage_events backlog.';

-- Serves the oldest-failure probe (ORDER BY inserted_at ASC LIMIT 1).
CREATE INDEX idx_usage_events_publish_failures_inserted_at ON usage_events_publish_failures (inserted_at);

-- Serves pruning of failure rows whose events aged out of the publish
-- window.
CREATE INDEX idx_usage_events_publish_failures_created_at ON usage_events_publish_failures (created_at);

-- Publishing an event resolves its failure row at the schema level so
-- every writer participates: during a rolling upgrade, a replica running
-- an older release can successfully publish an event whose failure row a
-- newer replica recorded, and its pre-upgrade post-publish update knows
-- nothing about this relation. Without the trigger that stale row would
-- keep raising a publish-failure warning until the event aged out of the
-- publish window. Cost is one primary-key delete per newly published row,
-- bounded by the publish batch size.
CREATE FUNCTION delete_usage_events_publish_failure() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    DELETE FROM usage_events_publish_failures WHERE event_id = NEW.id;
    RETURN NULL;
END;
$$;

CREATE TRIGGER trigger_delete_usage_events_publish_failure
    AFTER UPDATE OF published_at ON usage_events
    FOR EACH ROW
    WHEN (NEW.published_at IS NOT NULL AND OLD.published_at IS NULL)
    EXECUTE FUNCTION delete_usage_events_publish_failure();
