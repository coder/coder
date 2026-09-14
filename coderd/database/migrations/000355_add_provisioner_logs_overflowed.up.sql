 -- Add logs length tracking and overflow flag, similar to workspace agents
  ALTER TABLE provisioner_jobs ADD COLUMN logs_length integer NOT NULL DEFAULT 0 CONSTRAINT max_provisioner_logs_length CHECK (logs_length <= 1048576);
  ALTER TABLE provisioner_jobs ADD COLUMN logs_overflowed boolean NOT NULL DEFAULT false;

  COMMENT ON COLUMN provisioner_jobs.logs_length IS 'Total length of provisioner logs';
  COMMENT ON COLUMN provisioner_jobs.logs_overflowed IS 'Whether the provisioner logs overflowed in length';
