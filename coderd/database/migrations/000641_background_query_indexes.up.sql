-- AcquireNotificationMessages reads the oldest acquirable messages. Indexing
-- only queued rows lets it walk created_at order and stop at the batch size
-- instead of fetching and sorting every queued row. Sent and failed rows,
-- which make up almost all of the table, are not indexed.
CREATE INDEX idx_notification_messages_queued_created_at
	ON notification_messages (created_at)
	WHERE status IN ('pending', 'temporary_failure', 'leased');

-- GetProvisionerJobsToBeReaped looks for pending or running jobs that stopped
-- updating. Indexing only incomplete jobs avoids scanning the whole table.
-- The key is immutable so job heartbeats, which only touch updated_at, stay
-- eligible for HOT updates.
CREATE INDEX idx_provisioner_jobs_incomplete
	ON provisioner_jobs (created_at)
	WHERE completed_at IS NULL;

-- GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisioner
-- returns the newest jobs of an organization.
CREATE INDEX idx_provisioner_jobs_organization_id_created_at
	ON provisioner_jobs (organization_id, created_at DESC);
