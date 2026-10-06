-- An app in the shape every row has after 000596: the callback is the first
-- redirect URI.
INSERT INTO oauth2_provider_apps
	(id, created_at, updated_at, name, icon, callback_url, redirect_uris)
VALUES (
	'c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'2026-09-16 10:00:00+00',
	'2026-09-16 10:00:00+00',
	'oauth2-app-redirect-uris',
	'',
	'https://app.example.com/callback',
	'{https://app.example.com/callback,http://127.0.0.1/callback}'
);
