-- Improve provisioner daemons listing performance
-- Index 1: current (open) job lookup by worker
CREATE INDEX IF NOT EXISTS idx_provisioner_jobs_worker_open
ON provisioner_jobs USING btree (organization_id, worker_id)
WHERE completed_at IS NULL;

-- Index 2: most recent completed job per worker (order by completed_at desc)
CREATE INDEX IF NOT EXISTS idx_provisioner_jobs_worker_completed_at_desc
ON provisioner_jobs USING btree (worker_id, organization_id, completed_at DESC)
WHERE completed_at IS NOT NULL;

-- Index 3: satisfy ORDER BY for newest daemons per organization
CREATE INDEX IF NOT EXISTS idx_provisioner_daemons_org_created_at
ON provisioner_daemons USING btree (organization_id, created_at DESC);
