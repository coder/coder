DROP TABLE IF EXISTS task_snapshots;

-- Note: Cannot remove enum values in PostgreSQL.
-- The build_reason enum values (task_auto_pause, task_manual_pause, task_resume)
-- will remain but become unused.
