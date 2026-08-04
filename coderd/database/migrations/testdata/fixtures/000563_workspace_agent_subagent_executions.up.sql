INSERT INTO workspace_agents (
	id,
	created_at,
	updated_at,
	name,
	resource_id,
	auth_token,
	architecture,
	operating_system,
	parent_id,
	execution_isolation
)
SELECT
	'3d7ba76a-82ab-4a72-b18c-3ba3404a0a46'::uuid,
	'2022-11-02 13:03:45+02'::timestamptz,
	'2022-11-02 13:03:45+02'::timestamptz,
	'subagent-fixture',
	parent.resource_id,
	'02f6eb71-3d5d-45b0-99f4-108624f5cf5e'::uuid,
	parent.architecture,
	parent.operating_system,
	parent.id,
	TRUE
FROM workspace_agents AS parent
JOIN workspace_resources AS wr
	ON wr.id = parent.resource_id
JOIN workspace_builds AS wb
	ON wb.job_id = wr.job_id
WHERE parent.parent_id IS NULL
ORDER BY wb.created_at, wb.id, parent.created_at, parent.id
LIMIT 1
ON CONFLICT DO NOTHING;

INSERT INTO workspace_agent_subagent_executions (
	workspace_build_id,
	declaration_id,
	parent_agent_id,
	child_agent_id,
	driver,
	driver_protocol,
	shared_host_path,
	shared_child_path,
	startup_timeout_seconds,
	restart_policy
)
SELECT
	wb.id,
	'b8510489-fbf8-4443-bfee-bbb3c626d3a8'::uuid,
	parent.id,
	child.id,
	'claude-code',
	1,
	'/tmp/coder-subagent',
	'/tmp/coder-subagent',
	60,
	'never'
FROM workspace_agents AS parent
JOIN workspace_agents AS child
	ON child.parent_id = parent.id
JOIN workspace_resources AS wr
	ON wr.id = parent.resource_id
JOIN workspace_builds AS wb
	ON wb.job_id = wr.job_id
WHERE child.id = '3d7ba76a-82ab-4a72-b18c-3ba3404a0a46'::uuid
ON CONFLICT DO NOTHING;
