INSERT INTO oauth2_provider_apps
	(id, created_at, updated_at, name, icon, callback_url)
VALUES (
	'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'2023-06-15 10:23:54+00',
	'2023-06-15 10:23:54+00',
	'oauth2-app',
	'/some/icon.svg',
	'http://coder.com/oauth2/callback'
);

INSERT INTO oauth2_provider_app_secrets
	(id, created_at, last_used_at, hashed_secret, display_secret, app_id)
VALUES (
	'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'2023-06-15 10:25:33+00',
	'2023-12-15 11:40:20+00',
	CAST('abcdefg' AS bytea),
	'fg',
	'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'
);
