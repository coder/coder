INSERT INTO
	template_usage_stats (
		start_time,
		end_time,
		template_id,
		user_id,
		median_latency_ms,
		usage_mins,
		ssh_mins,
		sftp_mins,
		reconnecting_pty_mins,
		vscode_mins,
		jetbrains_mins,
		app_usage_mins
	)
VALUES
	(
		date_trunc('hour', NOW()),
		date_trunc('hour', NOW()) + '30 minute'::interval,
		gen_random_uuid(),
		gen_random_uuid(),
		45.342::real,
		30, -- usage
		30, -- ssh
		5, -- sftp
		2, -- reconnecting_pty
		10, -- vscode
		10, -- jetbrains
		'{"[terminal]": 2, "code-server": 30}'::jsonb
	);
