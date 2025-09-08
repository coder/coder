CREATE INDEX provisioner_jobs_worker_id_organization_id_completed_at_idx ON provisioner_jobs (worker_id, organization_id, completed_at DESC NULLS FIRST);

COMMENT ON INDEX provisioner_jobs_worker_id_organization_id_completed_at_idx IS 'Support index for finding the latest completed jobs for a worker (and organization), nulls first so that active jobs have priority; targets: GetProvisionerDaemonsWithStatusByOrganization';
