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

-- Tracks recent permanent rejections, keyed by event ID. Publish failure
-- detection reports rejections within the failure threshold; reading them
-- from this tiny indexed relation keeps the probe off the usage_events
-- table, where the successful-publish workload would otherwise be scanned
-- (the existing usage_events index does not cover failure_message, and
-- building a new index there would rewrite or scan a potentially huge
-- table inside the migration transaction).
CREATE TABLE usage_events_publish_rejections (
    event_id text PRIMARY KEY REFERENCES usage_events (id) ON DELETE CASCADE,
    -- Mirrors usage_events.published_at: when tallyman permanently
    -- rejected the event. Serves the recent-rejection probe and pruning.
    published_at timestamp with time zone NOT NULL
);

COMMENT ON TABLE usage_events_publish_rejections IS 'Usage events tallyman permanently rejected. Maintained by a trigger on usage_events; read by publish failure detection to find recent rejections without scanning usage_events.';

CREATE INDEX idx_usage_events_publish_rejections_published_at ON usage_events_publish_rejections (published_at);

-- Publishing an event resolves its failure row and records any permanent
-- rejection at the schema level so every writer participates: during a
-- rolling upgrade, a replica running an older release can publish or
-- permanently reject an event, and its pre-upgrade post-publish update
-- knows nothing about these relations. Without the trigger a stale
-- failure row would keep raising a publish-failure warning until the
-- event aged out of the publish window, and an old writer's permanent
-- rejection would go undetected. Cost is one primary-key delete (plus one
-- insert for rejections) per newly published row, bounded by the publish
-- batch size.
CREATE FUNCTION record_usage_events_publish_outcome() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    DELETE FROM usage_events_publish_failures WHERE event_id = NEW.id;
    IF NEW.failure_message IS NOT NULL THEN
        INSERT INTO usage_events_publish_rejections (event_id, published_at)
        VALUES (NEW.id, NEW.published_at)
        ON CONFLICT (event_id) DO NOTHING;
    END IF;
    RETURN NULL;
END;
$$;

CREATE TRIGGER trigger_record_usage_events_publish_outcome
    AFTER UPDATE OF published_at ON usage_events
    FOR EACH ROW
    WHEN (NEW.published_at IS NOT NULL AND OLD.published_at IS NULL)
    EXECUTE FUNCTION record_usage_events_publish_outcome();

-- A concluded publish attempt that leaves the event unpublished with a
-- failure message is a temporary failure; record it at the schema level so
-- writers running an older release also gain failure rows during a rolling
-- upgrade. The condition requires an in-flight attempt to conclude
-- (publish_started_at transitioning to NULL), which is how every release's
-- post-publish update reports temporary failures.
CREATE FUNCTION record_usage_events_publish_failure() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    INSERT INTO usage_events_publish_failures (event_id, inserted_at, created_at)
    VALUES (NEW.id, NEW.inserted_at, NEW.created_at)
    ON CONFLICT (event_id) DO NOTHING;
    RETURN NULL;
END;
$$;

CREATE TRIGGER trigger_record_usage_events_publish_failure
    AFTER UPDATE OF publish_started_at ON usage_events
    FOR EACH ROW
    WHEN (
        NEW.published_at IS NULL
        AND NEW.failure_message IS NOT NULL
        AND OLD.publish_started_at IS NOT NULL
        AND NEW.publish_started_at IS NULL
    )
    EXECUTE FUNCTION record_usage_events_publish_failure();
