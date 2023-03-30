-- On tables where data loss isn't a massive deal, we can make them unlogged
-- to dramatically improve performance.
ALTER TABLE workspace_agent_stats SET UNLOGGED;
ALTER TABLE workspace_agent_startup_logs SET UNLOGGED;
ALTER TABLE provisioner_job_logs SET UNLOGGED;
