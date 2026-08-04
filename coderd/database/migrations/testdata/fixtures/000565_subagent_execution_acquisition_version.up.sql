-- Exercise a positive acquisition_version on the declaration backfilled by
-- migration 564.
UPDATE workspace_agent_subagent_execution_statuses
SET acquisition_version = 2
WHERE declaration_id = 'b8510489-fbf8-4443-bfee-bbb3c626d3a8'::uuid;
