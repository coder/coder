-- Supports the permanent-rejection probe in GetUsagePublishStatus, which
-- every replica runs on each entitlements refresh. Without it, the probe
-- reads every event published within the failure threshold to test
-- failure_message, repeatedly scanning the normal successful-publish
-- workload. Permanent rejections are exceptional (a release gate keeps
-- unaccepted event types from being shipped), so the index stays near
-- empty and its creation writes almost nothing; the build's single heap
-- scan is the unavoidable cost of any index on this table.
CREATE INDEX idx_usage_events_permanent_rejections
    ON usage_events (published_at)
    WHERE published_at IS NOT NULL AND failure_message IS NOT NULL;
