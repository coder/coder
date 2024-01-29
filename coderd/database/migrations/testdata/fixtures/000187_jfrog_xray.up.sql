INSERT INTO jfrog_xray_scans
	(workspace_id, agent_id, critical, high, medium, results_url)
VALUES (
	gen_random_uuid(),
	gen_random_uuid(),
	10,
	5,
	2,
	'https://hello-world'
);

