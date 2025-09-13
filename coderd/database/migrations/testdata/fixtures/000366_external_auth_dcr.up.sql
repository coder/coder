INSERT INTO external_auth_dcr_clients (
    provider_id,
    client_id,
    client_secret,
    client_secret_key_id,
    created_at,
    updated_at
) VALUES (
    'coolsite',
    'dcr_client_id',
    'dcr_client_secret',
    'dcr_client_secret_key_id',
    '2025-01-01 00:00:00+00'::timestamptz,
    '2025-01-01 00:00:00+00'::timestamptz
);
