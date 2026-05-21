DROP TRIGGER IF EXISTS trigger_aggregate_usage_event ON usage_events;
DROP FUNCTION IF EXISTS aggregate_usage_event();
DROP TABLE IF EXISTS usage_events_daily;
