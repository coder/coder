INSERT INTO workspace_agent_stats (
	id,
	created_at,
	user_id,
	agent_id,
	workspace_id,
	template_id,
	connection_median_latency_ms
) VALUES (
	gen_random_uuid(),
	NOW(),
	gen_random_uuid(),
	gen_random_uuid(),
	gen_random_uuid(),
	gen_random_uuid(),
	1::bigint
);
