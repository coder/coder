-- Ensure api_keys and oauth2_provider_app_tokens have live data after
-- migration 000371 deletes expired rows.
INSERT INTO api_keys (
	id,
	hashed_secret,
	user_id,
	last_used,
	expires_at,
	created_at,
	updated_at,
	login_type,
	lifetime_seconds,
	ip_address,
	token_name,
	scopes,
	allow_list
)
VALUES (
	'fixture-api-key',
	'\xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
	'30095c71-380b-457a-8995-97b8ee6e5307',
	NOW() - INTERVAL '1 hour',
	NOW() + INTERVAL '30 days',
	NOW() - INTERVAL '1 day',
	NOW() - INTERVAL '1 day',
	'password',
	86400,
	'0.0.0.0',
	'fixture-api-key',
	ARRAY['workspace:read']::api_key_scope[],
	ARRAY['*:*']
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO oauth2_provider_app_tokens (
	id,
	created_at,
	expires_at,
	hash_prefix,
	refresh_hash,
	app_secret_id,
	api_key_id,
	audience,
	user_id
)
VALUES (
	'9f92f3c9-811f-4f6f-9a1c-3f2eed1f9f15',
	NOW() - INTERVAL '30 minutes',
	NOW() + INTERVAL '30 days',
	CAST('fixture-hash-prefix' AS bytea),
	CAST('fixture-refresh-hash' AS bytea),
	'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'fixture-api-key',
	'https://coder.example.com',
	'30095c71-380b-457a-8995-97b8ee6e5307'
)
ON CONFLICT (id) DO NOTHING;
