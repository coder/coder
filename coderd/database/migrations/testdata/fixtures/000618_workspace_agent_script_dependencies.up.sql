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
	resource_address,
	dependencies
) VALUES (
	'45e89705-e09d-4850-bcec-f9a937f5d78d',
	'2022-11-02 13:03:45.046432+02',
	'0ff953c0-92a6-4fe6-a415-eb0139a36ad1',
	'/tmp/install.log',
	'npm install',
	'',
	FALSE,
	TRUE,
	FALSE,
	0,
	'Install tools',
	'coder_script.install_tools',
	'[{"prerequisite_resource_address":"coder_script.clone_repo","requirement":"success"}]'
) ON CONFLICT DO NOTHING;
