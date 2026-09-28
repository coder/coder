INSERT INTO workspace_agent_stats (
	id, created_at, user_id, agent_id, workspace_id, template_id,
	connection_count, connection_median_latency_ms, session_counts
) VALUES (
	'4a382ba5-6e57-4a58-991e-d4ac4f6c1012', NOW(),
	gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), gen_random_uuid(),
	1, 1, '{"vscode": 2, "some_future_ide": 1}'::jsonb
);
