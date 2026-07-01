INSERT INTO workspace_agent_scripts (
	workspace_agent_id,
	created_at,
	log_source_id,
	log_path,
	script,
	cron,
	start_blocks_login,
	run_on_start,
	run_on_stop,
	timeout_seconds,
	display_name,
	id
) VALUES (
	'45e89705-e09d-4850-bcec-f9a937f5d78d',
	'2022-11-02 13:03:45.046432+02',
	'0ff953c0-92a6-4fe6-a415-eb0139a36ad1',
	'',
	'echo waits for the first script',
	'',
	false,
	true,
	false,
	0,
	'Ordered Script',
	'b8369ae5-d12b-4b31-bd5b-3e5e9a5f1d3a'
) ON CONFLICT DO NOTHING;

INSERT INTO workspace_agent_script_order (
	script_id,
	after_script_id,
	requires
) VALUES (
	'b8369ae5-d12b-4b31-bd5b-3e5e9a5f1d3a',
	(SELECT id FROM workspace_agent_scripts WHERE id != 'b8369ae5-d12b-4b31-bd5b-3e5e9a5f1d3a' LIMIT 1),
	'success'
) ON CONFLICT DO NOTHING;
