-- Add RFC 7591 Dynamic Client Registration fields to oauth2_provider_apps

-- RFC 7591 Core Fields
ALTER TABLE oauth2_provider_apps
    ADD COLUMN client_id_issued_at timestamptz DEFAULT NOW(),
    ADD COLUMN client_secret_expires_at timestamptz,
    ADD COLUMN grant_types text[] DEFAULT '{"authorization_code", "refresh_token"}',
    ADD COLUMN response_types text[] DEFAULT '{"code"}',
    ADD COLUMN token_endpoint_auth_method text DEFAULT 'client_secret_basic',
    ADD COLUMN scope text DEFAULT '',
    ADD COLUMN contacts text[];

-- RFC 7591 Optional Metadata Fields
ALTER TABLE oauth2_provider_apps
    ADD COLUMN client_uri text,
    ADD COLUMN logo_uri text,
    ADD COLUMN tos_uri text,
    ADD COLUMN policy_uri text;

-- RFC 7591 Advanced Fields
ALTER TABLE oauth2_provider_apps
    ADD COLUMN jwks_uri text,
    ADD COLUMN jwks jsonb,
    ADD COLUMN software_id text,
    ADD COLUMN software_version text;

-- RFC 7592 Management Fields
ALTER TABLE oauth2_provider_apps
    ADD COLUMN registration_access_token text,
    ADD COLUMN registration_client_uri text;

-- Backfill existing records with proper defaults
UPDATE oauth2_provider_apps SET
    client_id_issued_at = COALESCE(client_id_issued_at, created_at),
    grant_types = COALESCE(grant_types, '{"authorization_code", "refresh_token"}'),
    response_types = COALESCE(response_types, '{"code"}'),
    token_endpoint_auth_method = COALESCE(token_endpoint_auth_method, 'client_secret_basic'),
    scope = COALESCE(scope, ''),
    contacts = COALESCE(contacts, '{}')
WHERE client_id_issued_at IS NULL
   OR grant_types IS NULL
   OR response_types IS NULL
   OR token_endpoint_auth_method IS NULL
   OR scope IS NULL
   OR contacts IS NULL;

-- Add comments for documentation
COMMENT ON COLUMN oauth2_provider_apps.client_id_issued_at IS 'RFC 7591: Timestamp when client_id was issued';
COMMENT ON COLUMN oauth2_provider_apps.client_secret_expires_at IS 'RFC 7591: Timestamp when client_secret expires (null for non-expiring)';
COMMENT ON COLUMN oauth2_provider_apps.grant_types IS 'RFC 7591: Array of grant types the client is allowed to use';
COMMENT ON COLUMN oauth2_provider_apps.response_types IS 'RFC 7591: Array of response types the client supports';
COMMENT ON COLUMN oauth2_provider_apps.token_endpoint_auth_method IS 'RFC 7591: Authentication method for token endpoint';
COMMENT ON COLUMN oauth2_provider_apps.scope IS 'RFC 7591: Space-delimited scope values the client can request';
COMMENT ON COLUMN oauth2_provider_apps.contacts IS 'RFC 7591: Array of email addresses for responsible parties';
COMMENT ON COLUMN oauth2_provider_apps.client_uri IS 'RFC 7591: URL of the client home page';
COMMENT ON COLUMN oauth2_provider_apps.logo_uri IS 'RFC 7591: URL of the client logo image';
COMMENT ON COLUMN oauth2_provider_apps.tos_uri IS 'RFC 7591: URL of the client terms of service';
COMMENT ON COLUMN oauth2_provider_apps.policy_uri IS 'RFC 7591: URL of the client privacy policy';
COMMENT ON COLUMN oauth2_provider_apps.jwks_uri IS 'RFC 7591: URL of the client JSON Web Key Set';
COMMENT ON COLUMN oauth2_provider_apps.jwks IS 'RFC 7591: JSON Web Key Set document value';
COMMENT ON COLUMN oauth2_provider_apps.software_id IS 'RFC 7591: Identifier for the client software';
COMMENT ON COLUMN oauth2_provider_apps.software_version IS 'RFC 7591: Version of the client software';
COMMENT ON COLUMN oauth2_provider_apps.registration_access_token IS 'RFC 7592: Hashed registration access token for client management';
COMMENT ON COLUMN oauth2_provider_apps.registration_client_uri IS 'RFC 7592: URI for client configuration endpoint';
