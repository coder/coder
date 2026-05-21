INSERT INTO connection_logs (
	id,
	connect_time,
	organization_id,
	workspace_owner_id,
	workspace_id,
	workspace_name,
	agent_name,
	type,
	code,
	ip,
	user_agent,
	user_id,
	slug_or_port,
	connection_id,
	disconnect_time,
	disconnect_reason
) VALUES (
	'00000000-0000-0000-0000-000000000001', -- log id
	'2023-10-01 12:00:00+00', -- start time
	'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', -- organization id
	'a0061a8e-7db7-4585-838c-3116a003dd21', -- workspace owner id
	'3a9a1feb-e89d-457c-9d53-ac751b198ebe', -- workspace id
	'Test Workspace', -- workspace name
	'test-agent', -- agent name
	'ssh', -- type
	0, -- code
	'127.0.0.1', -- ip
	NULL, -- user agent
	NULL, -- user id
	NULL, -- slug or port
	'00000000-0000-0000-0000-000000000003', -- connection id
	'2023-10-01 12:00:10+00', -- close time
	'server shut down' -- reason
),
(
	'00000000-0000-0000-0000-000000000002', -- log id
	'2023-10-01 12:05:00+00', -- start time
	'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', -- organization id
	'a0061a8e-7db7-4585-838c-3116a003dd21', -- workspace owner id
	'3a9a1feb-e89d-457c-9d53-ac751b198ebe', -- workspace id
	'Test Workspace', -- workspace name
	'test-agent', -- agent name
	'workspace_app', -- type
	200, -- code
	'127.0.0.1',
	'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.127 Safari/537.36',
	'a0061a8e-7db7-4585-838c-3116a003dd21', -- user id
	'code-server', -- slug or port
	NULL, -- connection id (request ID)
	NULL, -- close time
	NULL -- reason
);
