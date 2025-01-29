INSERT INTO
	workspace_agent_memory_resource_monitors (
		agent_id,
		enabled,
		threshold,
		created_at
	)
	VALUES (
		'5755e622-fadd-44ca-98da-5df070491844', -- uuid
		true,
		90,
		'2024-01-01 00:00:00'
	);

INSERT INTO
	workspace_agent_volume_resource_monitors (
		agent_id,
		path,
		enabled,
		threshold,
		created_at
	)
	VALUES (
		'5755e622-fadd-44ca-98da-5df070491844', -- uuid
		'/',
		true,
		90,
		'2024-01-01 00:00:00'
	);

