-- Add index on workspace_builds.initiator_id to optimize prebuild queries
-- This will dramatically improve performance for:
-- - GetPrebuildMetrics (called every 15 seconds)
-- - Any other queries using workspace_prebuild_builds view
-- - Provisioner job queue prioritization
CREATE INDEX idx_workspace_builds_initiator_id ON workspace_builds (initiator_id);
