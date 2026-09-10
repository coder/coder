INSERT INTO oauth2_provider_device_codes
	(id, created_at, expires_at, secret_prefix, hashed_secret, user_code, app_id, user_id, status, scope, resource_uri)
VALUES (
	'e0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'2026-09-02 10:23:54+00',
	'2026-09-02 10:38:54+00',
	CAST('devprefix1' AS bytea),
	CAST('devhashed1' AS bytea),
	'ABCD-EFGH',
	'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	NULL,
	'pending',
	'coder:all',
	NULL
), (
	'e0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12',
	'2026-09-02 10:24:54+00',
	'2026-09-02 10:39:54+00',
	CAST('devprefix2' AS bytea),
	CAST('devhashed2' AS bytea),
	'JKLM-NPQR',
	'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
	'0ed9befc-4911-4ccf-a8e2-559bf72daa94',
	'authorized',
	'coder:all',
	'https://coder.example.com'
);
