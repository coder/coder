INSERT INTO exit_nodes (
	id,
	organization_id,
	name,
	display_name,
	created_at,
	updated_at,
	deleted,
	token_hashed_secret,
	version,
	last_seen_at,
	wireguard_endpoints
) VALUES (
	'8d3f4c2a-0b7e-4b1a-9c6d-2f5e8a1b3c4d',
	'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1',
	'egress-us-east',
	'Egress US East',
	'2025-01-01 00:00:00+00',
	'2025-01-01 00:00:00+00',
	false,
	'\x6d792d73656372657421'::bytea,
	'v2.30.0',
	'2025-01-01 00:05:00+00',
	'{"203.0.113.10:41641"}'
);
