INSERT INTO agent_peering_ids
	(agent_id, peering_id)
VALUES (
	'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'\xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef'
);

INSERT INTO tailnet_peering_events
	(peering_id, event_type, src_peer_id, dst_peer_id, node, occurred_at)
VALUES (
	'\xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef',
	'added_tunnel',
	'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'a fake protobuf byte string',
	'2025-01-15 10:23:54+00'
);
