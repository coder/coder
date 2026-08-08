INSERT INTO workspace_agent_stats (
	id,
	created_at,
	user_id,
	agent_id,
	workspace_id,
	template_id,
	connection_count,
	connection_median_latency_ms
) VALUES (
	'4a382ba5-6e57-4a58-991e-d4ac4f6c1012',
	NOW(),
	gen_random_uuid(),
	gen_random_uuid(),
	gen_random_uuid(),
	gen_random_uuid(),
	1::bigint,
	1::bigint
);

INSERT INTO workspace_agent_session_counts (
	workspace_agent_stats_id,
	created_at,
	app_name,
	count
) VALUES
	('4a382ba5-6e57-4a58-991e-d4ac4f6c1012', NOW(), 'vscode', 2),
	('4a382ba5-6e57-4a58-991e-d4ac4f6c1012', NOW(), 'ssh', 1),
	('4a382ba5-6e57-4a58-991e-d4ac4f6c1012', NOW(), 'SomeFutureIDE', 1);
