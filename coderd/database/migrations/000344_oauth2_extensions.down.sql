-- Remove OAuth2 extension fields

-- Remove fields from oauth2_provider_apps
ALTER TABLE oauth2_provider_apps
    DROP COLUMN IF EXISTS redirect_uris,
    DROP COLUMN IF EXISTS client_type,
    DROP COLUMN IF EXISTS dynamically_registered;

-- Remove audience field from oauth2_provider_app_tokens
ALTER TABLE oauth2_provider_app_tokens
    DROP COLUMN IF EXISTS audience;

-- Remove PKCE and resource fields from oauth2_provider_app_codes
ALTER TABLE oauth2_provider_app_codes
    DROP COLUMN IF EXISTS code_challenge_method,
    DROP COLUMN IF EXISTS code_challenge,
    DROP COLUMN IF EXISTS resource_uri;
