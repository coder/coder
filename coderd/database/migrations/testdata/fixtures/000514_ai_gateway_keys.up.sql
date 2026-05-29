INSERT INTO ai_gateway_keys (
	id,
	created_at,
	name,
	secret_prefix,
	hashed_secret,
	last_used_at
) VALUES (
	'8b6f0a82-9a3a-4d2e-8c0c-2c9c9b9b1a01',
	'2026-05-21 00:00:00+00',
	'example-key',
	'cdr_1234567',
	'\x00'::bytea,
	NULL
);
