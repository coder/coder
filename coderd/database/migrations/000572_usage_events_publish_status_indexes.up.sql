-- Support bounded MIN(inserted_at) probes in GetUsagePublishStatus, which
-- every replica runs on each entitlements refresh. Without these, a large
-- unpublished backlog (e.g. publishing re-enabled after weeks disabled) is
-- rescanned every refresh while the publisher drains it. Partial indexes
-- keep them small: unpublished rows are the exception.
CREATE INDEX idx_usage_events_unpublished_no_attempt
    ON usage_events (inserted_at)
    WHERE published_at IS NULL AND failure_message IS NULL;

CREATE INDEX idx_usage_events_unpublished_attempted
    ON usage_events (inserted_at)
    WHERE published_at IS NULL AND failure_message IS NOT NULL;
