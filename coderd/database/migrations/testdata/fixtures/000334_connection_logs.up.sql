INSERT INTO connection_logs (
	id,
	"time",
	connection_id,
	organization_id,
	workspace_owner_id,
	workspace_id,
	workspace_name,
	agent_name,
	action,
	code,
	ip,
	user_agent,
	user_id,
	slug_or_port,
	connection_type,
	reason
) VALUES (
	'00000000-0000-0000-0000-000000000001', -- log id
	'2023-10-01 12:00:00+00',
	'00000000-0000-0000-0000-000000000003', -- connection id
	'00000000-0000-0000-0000-000000000020', -- organization id
	'00000000-0000-0000-0000-000000000030', -- workspace owner id
	'3a9a1feb-e89d-457c-9d53-ac751b198ebe', -- workspace id
	'Test Workspace', -- workspace name
	'test-agent', -- agent name
	'connect',
	0, -- code
	'127.0.0.1',
	NULL, -- user agent
	'00000000-0000-0000-0000-000000000000', -- user id (uuid.Nil)
	NULL, -- slug or port
	'ssh', -- connection type
	'connected via CLI' -- reason
),
(
	'00000000-0000-0000-0000-000000000002', -- log id
	'2023-10-01 12:05:00+00',
	'00000000-0000-0000-0000-000000000004', -- connection id (request ID)
	'00000000-0000-0000-0000-000000000020', -- organization id
	'00000000-0000-0000-0000-000000000030', -- workspace owner id
	'3a9a1feb-e89d-457c-9d53-ac751b198ebe', -- workspace id
	'Test Workspace', -- workspace name
	'test-agent', -- agent name
	'open',
	200, -- code
	'127.0.0.1',
	'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.127 Safari/537.36',
	'00000000-0000-0000-0000-000000000030', -- user id
	'code-server', -- slug or port
	NULL, -- connection type
	NULL -- reason
);
