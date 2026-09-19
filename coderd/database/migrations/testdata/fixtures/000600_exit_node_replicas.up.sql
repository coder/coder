INSERT INTO exit_node_replicas (
	id,
	exit_node_id,
	hostname,
	version,
	wireguard_endpoints,
	policy_hash,
	created_at,
	started_at,
	updated_at,
	stopped_at
) VALUES (
	'2f8b7a31-9d4e-4e9f-a6c2-7b1d5e3f8a90',
	'8d3f4c2a-0b7e-4b1a-9c6d-2f5e8a1b3c4d',
	'exit-node-1',
	'v2.30.0',
	'{"203.0.113.10:41641"}',
	'sha256:fixture-policy',
	'2025-01-01 00:00:00+00',
	'2025-01-01 00:00:00+00',
	'2025-01-01 00:05:00+00',
	NULL
);
