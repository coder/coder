INSERT INTO provisioner_job_timings (job_id, started_at, ended_at, stage, source, action, resource)
VALUES
	-- Job 1 - init stage
	('d09b6083-e482-41ac-ad06-3aa731ec4fc6', NOW() - INTERVAL '1 hour 55 minutes', NOW() - INTERVAL '1 hour 50 minutes', 'init', 'source1', 'action1', 'resource1'),

	-- Job 1 - plan stage
	('d09b6083-e482-41ac-ad06-3aa731ec4fc6', NOW() - INTERVAL '1 hour 50 minutes', NOW() - INTERVAL '1 hour 40 minutes', 'plan', 'source2', 'action2', 'resource2'),

	-- Job 1 - graph stage
	('d09b6083-e482-41ac-ad06-3aa731ec4fc6', NOW() - INTERVAL '1 hour 40 minutes', NOW() - INTERVAL '1 hour 30 minutes', 'graph', 'source3', 'action3', 'resource3'),

	-- Job 1 - apply stage
	('d09b6083-e482-41ac-ad06-3aa731ec4fc6', NOW() - INTERVAL '1 hour 30 minutes', NOW() - INTERVAL '1 hour 20 minutes', 'apply', 'source4', 'action4', 'resource4');
