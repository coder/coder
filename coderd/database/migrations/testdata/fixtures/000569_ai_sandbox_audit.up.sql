INSERT INTO ai_sandbox_sessions (
	id,
	workspace_id,
	reporter_agent_id,
	confined_agent_id,
	ai_agent_id,
	sponsor_user_id,
	egress_enforcement,
	started_at,
	ended_at,
	created_at
) VALUES (
	'f47ac10b-58cc-4372-a567-0e02b2c3d479',
	'0a8f2b61-3c5d-4e7f-9a1b-2c3d4e5f6a7b',
	'1b9e3c72-4d6e-5f80-ab2c-3d4e5f6a7b8c',
	'1b9e3c72-4d6e-5f80-ab2c-3d4e5f6a7b8c',
	'2caf4d83-5e7f-6091-bc3d-4e5f6a7b8c9d',
	'3dbf5e94-6f80-71a2-cd4e-5f6a7b8c9d0e',
	'forced',
	'2024-01-01 00:00:00+00',
	NULL,
	'2024-01-01 00:00:00+00'
);

INSERT INTO ai_sandbox_network_events (
	session_id,
	occurred_at,
	protocol,
	host,
	port,
	action,
	policy_revision,
	ai_agent_id,
	sponsor_user_id,
	created_at
) VALUES (
	'f47ac10b-58cc-4372-a567-0e02b2c3d479',
	'2024-01-01 00:00:01+00',
	'connect',
	'example.com',
	443,
	'denied',
	0,
	'2caf4d83-5e7f-6091-bc3d-4e5f6a7b8c9d',
	'3dbf5e94-6f80-71a2-cd4e-5f6a7b8c9d0e',
	'2024-01-01 00:00:01+00'
);
