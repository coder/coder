-- A file-transfer session row (connect/disconnect pair) and two file
-- operation rows sharing its connection_id. The operation rows exercise
-- the partial unique index: they would collide with the session row and
-- each other under the previous non-partial index.
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
	disconnect_reason,
	file_protocol,
	file_action,
	file_path,
	file_target
) VALUES (
	'00000000-0000-0000-0000-000000000574', -- log id
	'2023-10-01 12:00:00+00', -- start time
	'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', -- organization id
	'a0061a8e-7db7-4585-838c-3116a003dd21', -- workspace owner id
	'3a9a1feb-e89d-457c-9d53-ac751b198ebe', -- workspace id
	'Test Workspace', -- workspace name
	'test-agent', -- agent name
	'file_transfer', -- type
	0, -- code
	'127.0.0.1', -- ip
	NULL, -- user agent
	NULL, -- user id
	NULL, -- slug or port
	'00000000-0000-0000-0000-000000000575', -- connection id
	'2023-10-01 12:00:10+00', -- close time
	'session ended', -- reason
	NULL, -- file protocol
	NULL, -- file action
	NULL, -- file path
	NULL -- file target
),
(
	'00000000-0000-0000-0000-000000000576', -- log id
	'2023-10-01 12:00:01+00', -- start time
	'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', -- organization id
	'a0061a8e-7db7-4585-838c-3116a003dd21', -- workspace owner id
	'3a9a1feb-e89d-457c-9d53-ac751b198ebe', -- workspace id
	'Test Workspace', -- workspace name
	'test-agent', -- agent name
	'file_operation', -- type
	NULL, -- code
	'127.0.0.1', -- ip
	NULL, -- user agent
	NULL, -- user id
	NULL, -- slug or port
	'00000000-0000-0000-0000-000000000575', -- connection id
	NULL, -- close time
	NULL, -- reason
	'sftp', -- file protocol
	'download', -- file action
	'/home/coder/secrets.txt', -- file path
	NULL -- file target
),
(
	'00000000-0000-0000-0000-000000000577', -- log id
	'2023-10-01 12:00:02+00', -- start time
	'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', -- organization id
	'a0061a8e-7db7-4585-838c-3116a003dd21', -- workspace owner id
	'3a9a1feb-e89d-457c-9d53-ac751b198ebe', -- workspace id
	'Test Workspace', -- workspace name
	'test-agent', -- agent name
	'file_operation', -- type
	NULL, -- code
	'127.0.0.1', -- ip
	NULL, -- user agent
	NULL, -- user id
	NULL, -- slug or port
	'00000000-0000-0000-0000-000000000575', -- connection id
	NULL, -- close time
	NULL, -- reason
	'sftp', -- file protocol
	'rename', -- file action
	'/home/coder/a.txt', -- file path
	'/home/coder/b.txt' -- file target
);
