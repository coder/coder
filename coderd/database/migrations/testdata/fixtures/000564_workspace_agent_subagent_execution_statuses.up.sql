-- Exercise mutable status fields on the declaration backfilled by migration 564.
UPDATE workspace_agent_subagent_execution_statuses
SET
	status = 'running',
	updated_at = '2022-11-02 13:04:45+02'::timestamptz,
	status_changed_at = '2022-11-02 13:04:45+02'::timestamptz,
	last_acquired_at = '2022-11-02 13:04:00+02'::timestamptz,
	last_reported_at = '2022-11-02 13:04:45+02'::timestamptz,
	restart_count = 1
WHERE declaration_id = 'b8510489-fbf8-4443-bfee-bbb3c626d3a8'::uuid;
