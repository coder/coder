-- Backfilled rows carry a family name as their app name, because the fixed
-- columns never recorded one. Real app names appear only in buckets the rollup
-- wrote after this migration, so seed one of each.
INSERT INTO template_usage_stats (
	start_time,
	end_time,
	template_id,
	user_id,
	median_latency_ms,
	usage_mins,
	app_usage_mins
) VALUES (
	date_trunc('hour', NOW()) + '30 minute'::interval,
	date_trunc('hour', NOW()) + '60 minute'::interval,
	'8f2b1c9e-6d4a-4f3b-8a7c-1e5d9b3a2c40',
	'2c7e5a1b-9d38-4c6f-b2e4-7a1f8c3d5b90',
	1,
	2,
	NULL
);

INSERT INTO template_usage_stats_session_apps (
	start_time,
	template_id,
	user_id,
	app_name,
	usage_mins
) VALUES (
	date_trunc('hour', NOW()) + '30 minute'::interval,
	'8f2b1c9e-6d4a-4f3b-8a7c-1e5d9b3a2c40',
	'2c7e5a1b-9d38-4c6f-b2e4-7a1f8c3d5b90',
	'vscode',
	2
), (
	date_trunc('hour', NOW()) + '30 minute'::interval,
	'8f2b1c9e-6d4a-4f3b-8a7c-1e5d9b3a2c40',
	'2c7e5a1b-9d38-4c6f-b2e4-7a1f8c3d5b90',
	'cursor',
	2
);
