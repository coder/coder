-- INSERT INTO workspace_agents VALUES ('45e89705-e09d-4850-bcec-f9a937f5d78d', '2022-11-02 13:03:45.046432+02', '2022-11-02 13:03:45.046432+02', 'main', NULL, NULL, NULL, '0ff953c0-92a6-4fe6-a415-eb0139a36ad1', 'ffc107ef-7ded-4d80-b1a9-0c1d0bf7ccbf', NULL, 'amd64', '{"GIT_AUTHOR_NAME": "default", "GIT_AUTHOR_EMAIL": "", "GIT_COMMITTER_NAME": "default", "GIT_COMMITTER_EMAIL": ""}', 'linux', 'code-server --auth none', NULL, NULL, '', '') ON CONFLICT DO NOTHING;

INSERT INTO workspace_agent_log_sources (
	workspace_agent_id,
	id,
	created_at,
	display_name,
	icon
) VALUES (
	'45e89705-e09d-4850-bcec-f9a937f5d78d',
	'0ff953c0-92a6-4fe6-a415-eb0139a36ad1',
	'2022-11-02 13:03:45.046432+02',
	'main',
	'something.png'
) ON CONFLICT DO NOTHING;

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
	timeout_seconds
) VALUES (
	'45e89705-e09d-4850-bcec-f9a937f5d78d',
	'2022-11-02 13:03:45.046432+02',
	'0ff953c0-92a6-4fe6-a415-eb0139a36ad1',
	'/tmp',
	'echo "hello world"',
	'@daily',
	TRUE,
	TRUE,
	TRUE,
	60
) ON CONFLICT DO NOTHING;
