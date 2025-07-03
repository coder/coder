-- Add purged_at column to workspace_builds table to track when logs/timings were purged
ALTER TABLE workspace_builds ADD COLUMN purged_at timestamptz;
