INSERT INTO
	workspace_agent_memory_resource_monitors (
		agent_id,
		enabled,
		threshold,
		created_at
	)
	VALUES (
		'45e89705-e09d-4850-bcec-f9a937f5d78d', -- uuid
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
		'45e89705-e09d-4850-bcec-f9a937f5d78d', -- uuid
		'/',
		true,
		90,
		'2024-01-01 00:00:00'
	);

