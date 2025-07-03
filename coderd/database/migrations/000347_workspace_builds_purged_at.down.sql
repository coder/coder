-- Remove purged_at column from workspace_builds table
ALTER TABLE workspace_builds DROP COLUMN purged_at;
