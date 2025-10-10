INSERT INTO oauth2_provider_device_codes (
    id, created_at, expires_at, device_code_hash, device_code_prefix,
    user_code, client_id, user_id, status, verification_uri,
    verification_uri_complete, scope, resource_uri, polling_interval
) VALUES (
    'c1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
    '2023-06-15 10:23:54+00',
    '2023-06-15 10:33:54+00',
    CAST('abcdefg123' AS bytea),
    'abcdefg1',
    'ABCD-1234',
    'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    'pending',
    'http://coder.com/oauth2/device',
    'http://coder.com/oauth2/device?user_code=ABCD1234',
    'read:user',
    'http://coder.com/api',
    5
);
