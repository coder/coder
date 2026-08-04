-- The constraint name abbreviates "acquisition_version" so the identifier stays
-- within PostgreSQL's 63-character limit.
ALTER TABLE workspace_agent_subagent_execution_statuses
	ADD COLUMN acquisition_version BIGINT NOT NULL DEFAULT 0,
	ADD CONSTRAINT workspace_agent_subagent_execution_statuses_acq_version_check
		CHECK (acquisition_version >= 0);

COMMENT ON COLUMN workspace_agent_subagent_execution_statuses.acquisition_version IS
	'Monotonically increasing counter bumped on each acquisition, fencing stale launchers of the same declaration.';
