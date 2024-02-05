INSERT INTO tailnet_peers
	(id, coordinator_id, updated_at, node, status)
VALUES (
	'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'2023-06-15 10:23:54+00',
	'a fake protobuf byte string',
	'ok'
);

INSERT INTO tailnet_tunnels
	(coordinator_id, src_id, dst_id, updated_at)
VALUES (
	'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'2023-06-15 10:23:54+00'
);
