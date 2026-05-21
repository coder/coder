INSERT INTO tailnet_coordinators
	(id, heartbeat_at)
VALUES
	(
 		'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
 		'2023-06-15 10:23:54+00'
 	);

INSERT INTO tailnet_clients
	(id, agent_id, coordinator_id, updated_at, node)
VALUES
	(
		'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
		'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
		'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
		'2023-06-15 10:23:54+00',
		'{"preferred_derp": 12}'::json
	);

INSERT INTO tailnet_agents
	(id, coordinator_id, updated_at, node)
VALUES
	(
		'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
		'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
		'2023-06-15 10:23:54+00',
		'{"preferred_derp": 13}'::json
	);
