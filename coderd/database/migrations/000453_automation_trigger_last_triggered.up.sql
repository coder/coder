ALTER TABLE automation_triggers ADD COLUMN last_triggered_at timestamp with time zone;

COMMENT ON COLUMN automation_triggers.last_triggered_at IS 'The last time this cron trigger was evaluated and fired. Used by the cron scheduler to determine which triggers are due.';
