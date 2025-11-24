-- Remove RFC 7591 Dynamic Client Registration fields from oauth2_provider_apps

-- Remove RFC 7592 Management Fields
ALTER TABLE oauth2_provider_apps
    DROP COLUMN IF EXISTS registration_access_token,
    DROP COLUMN IF EXISTS registration_client_uri;

-- Remove RFC 7591 Advanced Fields
ALTER TABLE oauth2_provider_apps
    DROP COLUMN IF EXISTS jwks_uri,
    DROP COLUMN IF EXISTS jwks,
    DROP COLUMN IF EXISTS software_id,
    DROP COLUMN IF EXISTS software_version;

-- Remove RFC 7591 Optional Metadata Fields
ALTER TABLE oauth2_provider_apps
    DROP COLUMN IF EXISTS client_uri,
    DROP COLUMN IF EXISTS logo_uri,
    DROP COLUMN IF EXISTS tos_uri,
    DROP COLUMN IF EXISTS policy_uri;

-- Remove RFC 7591 Core Fields
ALTER TABLE oauth2_provider_apps
    DROP COLUMN IF EXISTS client_id_issued_at,
    DROP COLUMN IF EXISTS client_secret_expires_at,
    DROP COLUMN IF EXISTS grant_types,
    DROP COLUMN IF EXISTS response_types,
    DROP COLUMN IF EXISTS token_endpoint_auth_method,
    DROP COLUMN IF EXISTS scope,
    DROP COLUMN IF EXISTS contacts;
