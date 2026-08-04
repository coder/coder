CREATE INDEX workspace_agents_parent_id_not_deleted_idx
ON workspace_agents (parent_id)
WHERE deleted = FALSE;

CREATE TABLE workspace_agent_subagent_execution_statuses (
	workspace_build_id UUID NOT NULL,
	declaration_id UUID NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	status_changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	last_acquired_at TIMESTAMPTZ,
	last_reported_at TIMESTAMPTZ,
	restart_count INTEGER NOT NULL DEFAULT 0,
	last_error TEXT NOT NULL DEFAULT '',
	CONSTRAINT workspace_agent_subagent_execution_statuses_pkey
		PRIMARY KEY (workspace_build_id, declaration_id),
	CONSTRAINT workspace_agent_subagent_execution_statuses_execution_fkey
		FOREIGN KEY (workspace_build_id, declaration_id)
		REFERENCES workspace_agent_subagent_executions (workspace_build_id, declaration_id)
		ON DELETE CASCADE,
	CONSTRAINT workspace_agent_subagent_execution_statuses_status_check
		CHECK (status IN ('pending', 'starting', 'running', 'stopping', 'stopped', 'failed')),
	CONSTRAINT workspace_agent_subagent_execution_statuses_restart_count_check
		CHECK (restart_count >= 0),
	CONSTRAINT workspace_agent_subagent_execution_statuses_last_error_check
		CHECK (octet_length(last_error) <= 4096)
);

INSERT INTO workspace_agent_subagent_execution_statuses (
	workspace_build_id,
	declaration_id
)
SELECT
	workspace_build_id,
	declaration_id
FROM workspace_agent_subagent_executions;
