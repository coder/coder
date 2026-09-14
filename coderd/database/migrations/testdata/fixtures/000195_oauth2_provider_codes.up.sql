INSERT INTO oauth2_provider_app_codes
	(id, created_at, expires_at, secret_prefix, hashed_secret, user_id, app_id)
VALUES (
	'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'2023-06-15 10:23:54+00',
	'2023-06-15 10:23:54+00',
	CAST('abcdefg' AS bytea),
	CAST('abcdefg' AS bytea),
	'0ed9befc-4911-4ccf-a8e2-559bf72daa94',
	'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'
);

INSERT INTO oauth2_provider_app_tokens
	(id, created_at, expires_at, hash_prefix, refresh_hash, app_secret_id, api_key_id)
VALUES (
	'd0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'2023-06-15 10:25:33+00',
	'2023-12-15 11:40:20+00',
	CAST('gfedcba' AS bytea),
	CAST('abcdefg' AS bytea),
	'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'peuLZhMXt4'
);
