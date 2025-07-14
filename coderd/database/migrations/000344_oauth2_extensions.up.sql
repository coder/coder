-- Add OAuth2 extension fields for RFC 8707 resource indicators, PKCE, and dynamic client registration

-- Add resource_uri field to oauth2_provider_app_codes for RFC 8707 resource parameter
ALTER TABLE oauth2_provider_app_codes
    ADD COLUMN resource_uri text;

COMMENT ON COLUMN oauth2_provider_app_codes.resource_uri IS 'RFC 8707 resource parameter for audience restriction';

-- Add PKCE fields to oauth2_provider_app_codes
ALTER TABLE oauth2_provider_app_codes
    ADD COLUMN code_challenge text,
    ADD COLUMN code_challenge_method text;

COMMENT ON COLUMN oauth2_provider_app_codes.code_challenge IS 'PKCE code challenge for public clients';
COMMENT ON COLUMN oauth2_provider_app_codes.code_challenge_method IS 'PKCE challenge method (S256)';

-- Add audience field to oauth2_provider_app_tokens for token binding
ALTER TABLE oauth2_provider_app_tokens
    ADD COLUMN audience text;

COMMENT ON COLUMN oauth2_provider_app_tokens.audience IS 'Token audience binding from resource parameter';

-- Add fields to oauth2_provider_apps for future dynamic registration and redirect URI management
ALTER TABLE oauth2_provider_apps
    ADD COLUMN redirect_uris text[], -- Store multiple URIs for future use
    ADD COLUMN client_type text DEFAULT 'confidential', -- 'confidential' or 'public'
    ADD COLUMN dynamically_registered boolean DEFAULT false;

-- Backfill existing records with default values
UPDATE oauth2_provider_apps SET
    redirect_uris = COALESCE(redirect_uris, '{}'),
    client_type = COALESCE(client_type, 'confidential'),
    dynamically_registered = COALESCE(dynamically_registered, false)
WHERE redirect_uris IS NULL OR client_type IS NULL OR dynamically_registered IS NULL;

COMMENT ON COLUMN oauth2_provider_apps.redirect_uris IS 'List of valid redirect URIs for the application';
COMMENT ON COLUMN oauth2_provider_apps.client_type IS 'OAuth2 client type: confidential or public';
COMMENT ON COLUMN oauth2_provider_apps.dynamically_registered IS 'Whether this app was created via dynamic client registration';
