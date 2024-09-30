INSERT INTO provisioner_job_timings (job_id, started_at, ended_at, stage, source, action, resource)
VALUES
	-- Job 1 - init stage
	('424a58cb-61d6-4627-9907-613c396c4a38', NOW() - INTERVAL '1 hour 55 minutes', NOW() - INTERVAL '1 hour 50 minutes', 'init', 'source1', 'action1', 'resource1'),

	-- Job 1 - plan stage
	('424a58cb-61d6-4627-9907-613c396c4a38', NOW() - INTERVAL '1 hour 50 minutes', NOW() - INTERVAL '1 hour 40 minutes', 'plan', 'source2', 'action2', 'resource2'),

	-- Job 1 - graph stage
	('424a58cb-61d6-4627-9907-613c396c4a38', NOW() - INTERVAL '1 hour 40 minutes', NOW() - INTERVAL '1 hour 30 minutes', 'graph', 'source3', 'action3', 'resource3'),

	-- Job 1 - apply stage
	('424a58cb-61d6-4627-9907-613c396c4a38', NOW() - INTERVAL '1 hour 30 minutes', NOW() - INTERVAL '1 hour 20 minutes', 'apply', 'source4', 'action4', 'resource4');
