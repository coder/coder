CREATE TABLE external_auth_dcr_clients (
    provider_id TEXT NOT NULL,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    client_secret_key_id TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    PRIMARY KEY (provider_id)
);

COMMENT ON TABLE external_auth_dcr_clients IS 'External authentication OAuth2 client details registered using dynamic client registration (e.g. public registration endpoint).';
COMMENT ON COLUMN external_auth_dcr_clients.client_secret_key_id IS 'The ID of the key used to encrypt the client secret. If this is NULL, the client secret is not encrypted.';
